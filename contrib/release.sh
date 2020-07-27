#!/bin/bash

# Script to do Dgraph release. This script would output the built binaries in
# $TMP.  This script should NOT be responsible for doing any testing, or
# uploading to any server.  The sole task of this script is to build the
# binaries and prepare them such that any human or script can then pick these up
# and use them as they deem fit.

# Path to this script
scriptdir="$(cd "$(dirname $0)">/dev/null; pwd)"
# Path to the root repo directory
repodir="$(cd "$scriptdir/..">/dev/null; pwd)"

# Output colors
RED='\033[91;1m'
RESET='\033[0m'

print_error() {
    printf "$RED$1$RESET\n"
}

exit_error() {
    print_error "$@"
    exit 1
}
check_command_exists() {
    if ! command -v "$1" > /dev/null; then
        exit_error "$1: command not found"
    fi
}

if [ "$#" -lt 1 ]; then
    exit_error "Usage: $0 commitish [docker_tag]

Examples:
Build v1.2.3 release binaries
  $0 v1.2.3
Build dev/feature-branch branch and tag as dev-abc123 for the Docker image
  $0 dev/feature-branch dev-abc123"
fi

export NVM_DIR="$HOME/.nvm"
[ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh"  # This loads nvm
[ -s "$NVM_DIR/bash_completion" ] && \. "$NVM_DIR/bash_completion"  # This loads nvm bash_completion

check_command_exists strip
check_command_exists make
check_command_exists gcc
check_command_exists go
check_command_exists docker
check_command_exists docker-compose
check_command_exists nvm
check_command_exists npm
check_command_exists protoc
check_command_exists shasum
check_command_exists tar
check_command_exists zip

# Don't use standard GOPATH. Create a new one.
unset GOBIN
export GOPATH="/tmp/go"
if [ -d $GOPATH ]; then
   chmod -R 755 $GOPATH
fi
rm -Rf $GOPATH
mkdir $GOPATH

# Necessary to pick up Gobin binaries like protoc-gen-gofast
PATH="$GOPATH/bin:$PATH"

# The Go version used for release builds must match this version.
GOVERSION="1.14.4"

TAG=$1
# The Docker tag should not contain a slash e.g. feature/issue1234
# The initial slash is taken from the repository name dgraph/dgraph:tag
DOCKER_TAG=${2:-$(echo "$TAG" | tr '/' '-')}

(
    cd "$repodir"
    git cat-file -e "$TAG"
) || exit_error "Ref $TAG does not exist"

# DO NOT change the /tmp/build directory, because Dockerfile also picks up binaries from there.
TMP="/tmp/build"
rm -Rf $TMP
mkdir $TMP

if [ -z "$TAG" ]; then
  echo "Must specify which tag to build for."
  exit 1
fi
echo "Building Dgraph for tag: $TAG"

# Stop on first failure.
set -e
set -o xtrace

ratel_release="github.com/dgraph-io/ratel/server.ratelVersion"
release="github.com/dgraph-io/dgraph/x.dgraphVersion"
branch="github.com/dgraph-io/dgraph/x.gitBranch"
commitSHA1="github.com/dgraph-io/dgraph/x.lastCommitSHA"
commitTime="github.com/dgraph-io/dgraph/x.lastCommitTime"

go install src.techknowlogick.com/xgo

basedir=$GOPATH/src/github.com/dgraph-io
mkdir -p "$basedir"

# Clone Dgraph repo.
pushd $basedir
  git clone "$repodir"
popd

pushd $basedir/dgraph
  git checkout $TAG
  # HEAD here points to whatever is checked out.
  lastCommitSHA1=$(git rev-parse --short HEAD)
  gitBranch=$(git rev-parse --abbrev-ref HEAD)
  lastCommitTime=$(git log -1 --format=%ci)
  release_version=$(git describe --always --tags)
popd

# Regenerate protos. Should not be different from what's checked in.
pushd $basedir/dgraph/protos
  # We need to fetch the modules to get the correct proto files. e.g., for
  # badger and dgo
  go get -d -v ../dgraph

  make regenerate
  if [[ "$(git status --porcelain)" ]]; then
      echo >&2 "Generated protos different in release."
      exit 1
  fi
popd

# Clone ratel repo.
pushd $basedir
  git clone https://github.com/dgraph-io/ratel.git
popd

pushd $basedir/ratel
  nvm install --lts
  (export GO111MODULE=off; ./scripts/build.prod.sh)
  ./scripts/test.sh
popd

# Clone Badger repo.
pushd $basedir
  git clone https://github.com/dgraph-io/badger.git
popd

# Build Windows.
pushd $basedir/dgraph/dgraph
  xgo -go="go-$GOVERSION" --targets=windows/amd64 -ldflags \
      "-X $release=$release_version -X $branch=$gitBranch -X $commitSHA1=$lastCommitSHA1 -X '$commitTime=$lastCommitTime'" .
  mkdir $TMP/windows
  mv dgraph-windows-4.0-amd64.exe $TMP/windows/dgraph.exe
popd

pushd $basedir/badger/badger
  xgo -go="go-$GOVERSION" --targets=windows/amd64 .
  mv badger-windows-4.0-amd64.exe $TMP/windows/badger.exe
popd

pushd $basedir/ratel
  xgo -go="go-$GOVERSION" --targets=windows/amd64 -ldflags "-X $ratel_release=$release_version" .
  mv ratel-windows-4.0-amd64.exe $TMP/windows/dgraph-ratel.exe
popd

# Build Darwin.
pushd $basedir/dgraph/dgraph
  xgo -go="go-$GOVERSION" --targets=darwin-10.9/amd64 -ldflags \
  "-X $release=$release_version -X $branch=$gitBranch -X $commitSHA1=$lastCommitSHA1 -X '$commitTime=$lastCommitTime'" .
  mkdir $TMP/darwin
  mv dgraph-darwin-10.9-amd64 $TMP/darwin/dgraph
popd

pushd $basedir/badger/badger
  xgo -go="go-$GOVERSION" --targets=darwin-10.9/amd64 .
  mv badger-darwin-10.9-amd64 $TMP/darwin/badger
popd

pushd $basedir/ratel
  xgo -go="go-$GOVERSION" --targets=darwin-10.9/amd64 -ldflags "-X $ratel_release=$release_version" .
  mv ratel-darwin-10.9-amd64 $TMP/darwin/dgraph-ratel
popd

# Build Linux.
pushd $basedir/dgraph/dgraph
  xgo -go="go-$GOVERSION" --targets=linux/amd64 -ldflags \
      "-X $release=$release_version -X $branch=$gitBranch -X $commitSHA1=$lastCommitSHA1 -X '$commitTime=$lastCommitTime'" .
  strip -x dgraph-linux-amd64
  mkdir $TMP/linux
  mv dgraph-linux-amd64 $TMP/linux/dgraph
popd

pushd $basedir/badger/badger
  xgo -go="go-$GOVERSION" --targets=linux/amd64 .
  strip -x badger-linux-amd64
  mv badger-linux-amd64 $TMP/linux/badger
popd

pushd $basedir/ratel
  xgo -go="go-$GOVERSION" --targets=linux/amd64 -ldflags "-X $ratel_release=$release_version" .
  strip -x ratel-linux-amd64
  mv ratel-linux-amd64 $TMP/linux/dgraph-ratel
popd

createSum () {
  os=$1
  echo "Creating checksum for $os"
  pushd $TMP/$os
    csum=$(shasum -a 256 dgraph | awk '{print $1}')
    echo $csum /usr/local/bin/dgraph >> ../dgraph-checksum-$os-amd64.sha256
    csum=$(shasum -a 256 dgraph-ratel | awk '{print $1}')
    echo $csum /usr/local/bin/dgraph-ratel >> ../dgraph-checksum-$os-amd64.sha256
  popd
}

createSum darwin
createSum linux
createSum windows

# Create Docker image.
cp $basedir/dgraph/contrib/Dockerfile $TMP
pushd $TMP
  docker build -t dgraph/dgraph:$DOCKER_TAG .
popd
rm $TMP/Dockerfile

# TODO Create Docker standalone image
pushd $basedir/dgraph/contrib/standalone
  make DGRAPH_VERSION=$DOCKER_TAG
popd

# Create the tar and delete the binaries.
createTar () {
  os=$1
  echo "Creating tar for $os"
  pushd $TMP/$os
    tar -zcvf ../dgraph-$os-amd64.tar.gz *
  popd
  rm -Rf $TMP/$os
}

# Create the zip and delete the binaries.
createZip () {
  os=$1
  echo "Creating zip for $os"
  pushd $TMP/$os
    zip -r ../dgraph-$os-amd64.zip *
  popd
  rm -Rf $TMP/$os
}

createZip windows
createTar darwin
createTar linux

echo "Release $TAG is ready."
docker run dgraph/dgraph:$DOCKER_TAG dgraph
ls -alh $TMP

set +o xtrace
echo "To release:"
echo
if git show-ref -q --verify "refs/tags/$TAG"; then
    echo "Push the git tag"
    echo "  git push origin $TAG"
fi
echo
echo "Push the Docker tag:"
echo "  docker push dgraph/dgraph:$DOCKER_TAG"
echo
echo "If this should be the latest release, then tag"
echo "the image as latest too."
echo "  docker tag dgraph/dgraph:$DOCKER_TAG dgraph/dgraph:latest"
echo "  docker push dgraph/dgraph:latest"
