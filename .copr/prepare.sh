#!/bin/sh -euf
set -x
OS_TEST=${OS_TEST:=0}

if [ ! -e /usr/bin/git ]; then
    dnf -y install git-core
fi

if [ -f $(git rev-parse --git-dir)/shallow ]; then
    git fetch --unshallow
fi

COMMIT=$(git rev-parse HEAD)
COMMIT_SHORT=$(git rev-parse --short=8 HEAD)
COMMIT_NUM=$(git rev-list HEAD --count)
COMMIT_DATE=$(date +%s)
BR="#BuildRequires: golang-bin"
NEWBR="BuildRequires: golang-bin"


sed "s,#COMMIT#,${COMMIT},;
     s,#SHORTCOMMIT#,${COMMIT_SHORT},;
     s,#COMMITNUM#,${COMMIT_NUM},;
     s,#COMMITDATE#,${COMMIT_DATE}," \
         contrib/spec/podman.spec.in > contrib/spec/podman.spec

if [ ${OS_TEST} -eq 0 ]; then
    sed -i "s/${BR}/${NEWBR}/g" contrib/spec/podman.spec
fi

mkdir -p build/
git archive --prefix "libpod-${COMMIT_SHORT}/" --format "tar.gz" HEAD -o "build/libpod-${COMMIT_SHORT}.tar.gz"
if [ ! -d conmon ]; then
    git clone -n --quiet https://github.com/containers/conmon
fi
pushd conmon
git checkout 6f3572558b97bc60dd8f8c7f0807748e6ce2c440
git archive --prefix "conmon/" --format "tar.gz" HEAD -o "../build/conmon.tar.gz"
popd
