#!/usr/bin/env bats   -*- bats -*-
#
# tests for podman load
#

load helpers

# Custom helpers for this test only. These just save us having to duplicate
# the same thing four times (two tests, each with -i and stdin).
#
# initialize, read image ID and name
get_iid_and_name() {
    run_podman images -a --format '{{.ID}} {{.Repository}}:{{.Tag}}'
    read iid img_name < <(echo "$output")

    archive=$PODMAN_TMPDIR/myimage-$(random_string 8).tar
}

# Simple verification of image ID and name
verify_iid_and_name() {
    run_podman images -a --format '{{.ID}} {{.Repository}}:{{.Tag}}'
    read new_iid new_img_name < <(echo "$output")

    # Verify
    is "$new_iid"      "$iid" "Image ID of loaded image == original"
    is "$new_img_name" "$1"   "Name & tag of restored image"
}

@test "podman save to pipe and load" {
    # We can't use run_podman because that uses the BATS 'run' function
    # which redirects stdout and stderr. Here we need to guarantee
    # that podman's stdout is a pipe, not any other form of redirection
    $PODMAN save --format oci-archive $IMAGE | cat >$PODMAN_TMPDIR/test.tar
    [ $status -eq 0 ]

    run_podman load -i $PODMAN_TMPDIR/test.tar
}


@test "podman load - by image ID" {
    skip_if_remote "FIXME: pending #7123"

    # FIXME: how to build a simple archive instead?
    get_iid_and_name

    # Save image by ID, and remove it.
    run_podman save $iid -o $archive
    run_podman rmi $iid

    # Load using -i; IID should be preserved, but name is not.
    run_podman load -i $archive
    verify_iid_and_name "<none>:<none>"

    # Same as above, using stdin
    run_podman rmi $iid
    run_podman load < $archive
    verify_iid_and_name "<none>:<none>"

    # Same as above, using stdin but with `podman image load`
    run_podman rmi $iid
    run_podman image load < $archive
    verify_iid_and_name "<none>:<none>"

    # Cleanup: since load-by-iid doesn't preserve name, re-tag it;
    # otherwise our global teardown will rmi and re-pull our standard image.
    run_podman tag $iid $img_name
}

@test "podman load - by image name" {
    get_iid_and_name
    run_podman save $img_name -o $archive
    run_podman rmi $iid

    # Load using -i; this time the image should be tagged.
    run_podman load -i $archive
    verify_iid_and_name $img_name
    run_podman rmi $iid

    # Also make sure that `image load` behaves the same.
    run_podman image load -i $archive
    verify_iid_and_name $img_name
    run_podman rmi $iid

    # Same as above, using stdin
    run_podman load < $archive
    verify_iid_and_name $img_name
}

@test "podman load - NAME and NAME:TAG arguments work" {
    get_iid_and_name
    run_podman save $iid -o $archive
    run_podman rmi $iid

    # Load with just a name (note: names must be lower-case)
    random_name=$(random_string 20 | tr A-Z a-z)
    run_podman load -i $archive $random_name
    verify_iid_and_name "localhost/$random_name:latest"

    # Load with NAME:TAG arg
    run_podman rmi $iid
    random_tag=$(random_string 10 | tr A-Z a-z)
    run_podman load -i $archive $random_name:$random_tag
    verify_iid_and_name "localhost/$random_name:$random_tag"

    # Cleanup: restore desired image name
    run_podman tag $iid $img_name
    run_podman rmi "$random_name:$random_tag"
}


@test "podman load - will not read from tty" {
    if [ ! -t 0 ]; then
        skip "STDIN is not a tty"
    fi

    run_podman 125 load
    is "$output" \
       "Error: cannot read from terminal. Use command-line redirection" \
       "Diagnostic from 'podman load' without redirection or -i"
}

@test "podman load - at most 1 arg(s)" {
    run_podman 125 load 1 2 3
    is "$output" \
       "Error: accepts at most 1 arg(s), received 3"
}

# vim: filetype=sh
