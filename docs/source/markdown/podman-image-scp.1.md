% podman-image-scp(1)

## NAME
podman-image-scp - Securely copy an image from one host to another

## SYNOPSIS
**podman image scp** [*options*] *name*[:*tag*]

## DESCRIPTION
**podman image scp** copies container images between hosts on a network. You can load to the remote host or from the remote host as well as in between two remote hosts.
Note: `::` is used to specify the output file on the host or the image name depending on if you are saving or loading.

**podman [GLOBAL OPTIONS]**

**podman imaege scp [GLOBAL OPTIONS]**

**podman image scp [OPTIONS] NAME[:TAG] HOSTNAME[::FILELOCATION]**

**podman image scp [OPTIONS] HOSTNAME[::IMAGENAME]**

## OPTIONS

#### **--format**=*format*

Save image to **docker-archive**, **oci-archive** (see `containers-transports(5)`).
```
--format docker-archive
--format oci-archive
```

#### **--quiet**, **-q**

Suppress the output

#### **--help**, **-h**

Print usage statement

## EXAMPLES

```
$ podman --remote image scp --quiet alpine:2.6
```

```
$ podman image scp --format oci-archive alpine
```

```
$ podman --remote image image scp alpine
Loaded image(s): docker.io/library/alpine:latest
```

```
$ podman image scp alpine --format oci-archive Fedora::/home/charliedoern/Documents/alpine
Getting image source signatures
Copying blob 72e830a4dff5 done
Copying config 85f9dc67c7 done
Writing manifest to image destination
Storing signatures
Loaded image(s): docker.io/library/alpine:latest
```

```
$ podman image scp Fedora::alpine Fedora::
Loaded image(s): docker.io/library/alpine:latest
```

```
$ podman image scp charliedoern@192.168.68.126:22/run/user/1000/podman/podman.sock::alpine
WARN[0000] Unknown connection name given. Please use system connection add to specify the default remote socket location
Getting image source signatures
Copying blob 9450ef9feb15 [--------------------------------------] 0.0b / 0.0b
Copying config 1f97f0559c done
Writing manifest to image destination
Storing signatures
Loaded images(s): localhost/alpine:latest
```

## SEE ALSO
podman(1), podman-load(1), podman-save(1), podman-remote(1), podman-system-connection-add(1), containers.conf(5), containers-transports(5)

## HISTORY
July 2021, Originally compiled by Charlie Doern <cdoern@redhat.com>
