#!/usr/bin/env bats
#
# Tests based on 'podman help'
#
# Find all commands listed by 'podman --help'. Run each one, make sure it
# provides its own --help output. If the usage message ends in '[command]',
# treat it as a subcommand, and recurse into its own list of sub-subcommands.
#
# Any usage message that ends in '[flags]' is interpreted as a command
# that takes no further arguments; we confirm by running with 'invalid-arg'
# and confirming that it exits with error status and message.
#
load helpers

# run 'podman help', parse the output looking for 'Available Commands';
# return that list.
function podman_commands() {
    dprint "$@"
    run_podman help "$@" |\
        awk '/^Available Commands:/{ok=1;next}/^Flags:/{ok=0}ok { print $1 }' |\
        grep .
    "$output"
}


function check_help() {
    local count=0
    local -A found

    for cmd in $(podman_commands "$@"); do
        # Human-readable podman command string, with multiple spaces collapsed
        command_string="podman $* $cmd"
        command_string=${command_string//  / }  # 'podman  x' -> 'podman x'

        dprint "$command_string --help"
        run_podman "$@" $cmd --help
        local full_help="$output"

        # The line immediately after 'Usage:' gives us a 1-line synopsis
        usage=$(echo "$full_help" | grep -A1 '^Usage:' | tail -1)
        [ -n "$usage" ] || die "podman $cmd: no Usage message found"

        # e.g. 'podman ps' should not show 'podman container ps' in usage
        # Trailing space in usage handles 'podman system renumber' which
        # has no ' [flags]'
        is "$usage " "  $command_string .*" "Usage string matches command"

        # If usage ends in '[command]', recurse into subcommands
        if expr "$usage" : '.*\[command\]$' >/dev/null; then
            found[subcommands]=1
            check_help "$@" $cmd
            continue
        fi

        # We had someone write upper-case '[FLAGS]' once. Prevent it.
        if expr "$usage" : '.*\[FLAG' >/dev/null; then
            die "'flags' string must be lower-case in usage: $usage"
        fi

        # We had someone do 'podman foo ARG [flags]' one time. Yeah, no.
        if expr "$usage" : '.*[A-Z].*\[flag' >/dev/null; then
            die "'flags' must precede arguments in usage: $usage"
        fi

        # Cross-check: if usage includes '[flags]', there must be a
        # longer 'Flags:' section in the full --help output; vice-versa,
        # if 'Flags:' is in full output, usage line must have '[flags]'.
        if expr "$usage" : '.*\[flag' >/dev/null; then
            if ! expr "$full_help" : ".*Flags:" >/dev/null; then
                die "$command_string: Usage includes '[flags]' but has no 'Flags:' subsection"
            fi
        elif expr "$full_help" : ".*Flags:" >/dev/null; then
            die "$command_string: --help has 'Flags:' section but no '[flags]' in synopsis"
        fi

        # If usage lists no arguments (strings in ALL CAPS), confirm
        # by running with 'invalid-arg' and expecting failure.
        if ! expr "$usage" : '.*[A-Z]' >/dev/null; then
            if [ "$cmd" != "help" ]; then
                dprint "$command_string invalid-arg"
                run_podman '?' "$@" $cmd invalid-arg
                is "$status" 125 "'$command_string invalid-arg' - exit status"
                is "$output" "Error: .* takes no arguments" \
                   "'$command_string' with extra (invalid) arguments"
            fi
            found[takes_no_args]=1
        fi

        # If usage has required arguments, try running without them.
        # The expression here is 'first capital letter is not in [BRACKETS]'.
        # It is intended to handle 'podman foo [flags] ARG' but not ' [ARG]'.
        if expr "$usage" : '[^A-Z]\+ [A-Z]' >/dev/null; then
            # Exceptions: these commands don't work rootless
            if is_rootless; then
                # "pause is not supported for rootless containers"
                if [ "$cmd" = "pause" -o "$cmd" = "unpause" ]; then
                    continue
                fi
                # "network rm" too
                if [ "$@" = "network" -a "$cmd" = "rm" ]; then
                    continue
                fi
            fi

            # The </dev/null protects us from 'podman login' which will
            # try to read username/password from stdin.
            dprint "$command_string (without required args)"
            run_podman '?' "$@" $cmd </dev/null
            is "$status" 125 "'$command_string' with no arguments - exit status"
            is "$output" "Error:.* \(require\|specif\|must\|provide\|need\|choose\|accepts\)" \
               "'$command_string' without required arg"

            found[required_args]=1
        fi

        # Commands with fixed number of arguments (i.e. no ellipsis): count
        # the required args, then invoke with one extra. We should get a
        # usage error.
        if ! expr "$usage" : ".*\.\.\."; then
            # "podman help" can take infinite args, so skip that one
            if [ "$cmd" != "help" ]; then
                # Get the args part of the command line; this should be
                # everything from the first CAPITAL LETTER onward. We
                # don't actually care about the letter itself, so just
                # make it 'X'. And we don't care about [OPTIONAL] brackets
                # either. What we do care about is stuff like 'IMAGE | CTR'
                # which is actually one argument; convert to 'IMAGE-or-CTR'
                local rhs=$(sed -e 's/^[^A-Z]\+[A-Z]/X/' -e 's/ | /-or-/g' <<<"$usage")
                local n_args=$(wc -w <<<"$rhs")

                run_podman '?' "$@" $cmd $(seq --format='x%g' 0 $n_args)
                is "$status" 125 "'$command_string' with >$n_args arguments - exit status"
                is "$output" "Error:.* \(takes no arguments\|requires exactly $n_args arg\|accepts at most\|too many arguments\|accepts $n_args arg(s), received\|accepts between .* and .* arg(s), received \)" \
                   "'$command_string' with >$n_args arguments"

                found[fixed_args]=1
            fi
        fi

        count=$(expr $count + 1)
    done

    # Any command that takes subcommands, must throw error if called
    # without one.
    dprint "podman $@"
    run_podman '?' "$@"
    is "$status" 125 "'podman $*' without any subcommand - exit status"
    is "$output" "Error: missing command .*$@ COMMAND" \
       "'podman $*' without any subcommand - expected error message"

    # Assume that 'NoSuchCommand' is not a command
    dprint "podman $@ NoSuchCommand"
    run_podman '?' "$@" NoSuchCommand
    is "$status" 125 "'podman $* NoSuchCommand' - exit status"
    is "$output" "Error: unrecognized command .*$@ NoSuchCommand" \
       "'podman $* NoSuchCommand' - expected error message"

    # This can happen if the output of --help changes, such as between
    # the old command parser and cobra.
    [ $count -gt 0 ] || \
        die "Internal error: no commands found in 'podman help $@' list"

    # Sanity check: make sure the special loops above triggered at least once.
    # (We've had situations where a typo makes the conditional never run)
    if [ -z "$*" ]; then
        for i in subcommands required_args takes_no_args fixed_args; do
            if [[ -z ${found[$i]} ]]; then
                die "Internal error: '$i' subtest did not trigger"
            fi
        done
    fi
}


@test "podman help - basic tests" {
    skip_if_remote

    # Called with no args -- start with 'podman --help'. check_help() will
    # recurse for any subcommands.
    check_help
}

# vim: filetype=sh
