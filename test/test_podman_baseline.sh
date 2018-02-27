#!/bin/bash
# test_podman_baseline.sh
# A script to be run at the command line with Podman installed.
# This should be run against a new kit to provide base level testing
# on a freshly installed machine with no images or container in
# play.  This currently needs to be run as root.
#
# To run this command:
#
# /bin/bash -v test_podman_baseline.sh -e # Stop on error
# /bin/bash -v test_podman_baseline.sh    # Continue on error

#######
# See if we want to stop on errors or not.
#######
showerror=0
while getopts "e" opt; do
    case "$opt" in
    e) showerror=1
       ;;
    esac
done

if [ "$showerror" -eq 1 ]
then
    echo "Script will stop on unexpected errors."
    set -eu
fi

########
# Next two commands should return blanks
########
podman images
podman ps --all

########
# Run ls in redis container, this should work
########
ctrid=$(podman pull registry.access.redhat.com/rhscl/redis-32-rhel7)
podman run $ctrid ls /

########
# Remove images and containers
########
podman rm --all
podman rmi --all

########
# Create Fedora based image
########
image=$(podman pull fedora)
echo $image

########
# Run container and display contents in /etc
########
podman run $image ls -alF /etc

########
# Run Java in the container - should ERROR but never stop
########
podman run $image java 2>&1 || echo $?

########
# Clean out containers
########
podman rm --all

########
# Install java onto the container, commit it, then run it showing java usage
########
podman run --net=host $image dnf -y install java
javaimage=$(podman ps --all -q)
podman commit $javaimage javaimage
podman run javaimage java

########
# Cleanup containers and images
########
podman rm --all
podman rmi --all

########
# Check images and containers, should be blanks
########
podman ps --all
podman images

########
# Create Fedora based container
########
image=$(podman pull fedora)
echo $image
podman run $image ls /

########
# Create shell script to test on
########
FILE=./runecho.sh
/bin/cat <<EOM >$FILE
#!/bin/bash
for i in {1..9};
do
    echo "This is a new container pull ipbabble [" \$i "]"
done
EOM
chmod +x $FILE

########
# Copy and run file on container
########
ctrid=$(podman ps --all -q)
mnt=$(podman mount $ctrid)
cp ./runecho.sh ${mnt}/tmp/runecho.sh
podman umount $ctrid
podman commit $ctrid runecho
podman run runecho ./tmp/runecho.sh

########
# Inspect the container, verifying above was put into it
########
podman inspect $ctrid

########
# Check the images there should be a runecho image
########
podman images

########
# Remove the containers
########
podman rm -a

########
# Install Docker, but not for long!
########
dnf -y install docker
systemctl start docker

########
# Push fedora-bashecho to the Docker daemon
########
podman push runecho docker-daemon:fedora-bashecho:latest

########
# Run fedora-bashecho pull Docker
########
docker run fedora-bashecho ./tmp/runecho.sh

########
# Time to remove Docker
########
dnf -y remove docker

########
# Build Dockerfile
########
FILE=./Dockerfile
/bin/cat <<EOM >$FILE
FROM docker/whalesay:latest
RUN apt-get -y update && apt-get install -y fortunes
CMD /usr/games/fortune -a | cowsay
EOM
chmod +x $FILE

########
# Build with the Dockerfile
########
podman build -f Dockerfile -t whale-says

########
# Run the container to see what the whale says
########
podman run whale-says

########
# Clean up Podman
########
podman rm --all
podman rmi --all
rm ./Dockerfile
