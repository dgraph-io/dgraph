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

## Toggle Builds
## TODO: update to use command line flags
DGRAPH_BUILD_WINDOWS=${DGRAPH_BUILD_WINDOWS:-0}
DGRAPH_BUILD_MAC=${DGRAPH_BUILD_MAC:-0}
DGRAPH_BUILD_RATEL=${DGRAPH_BUILD_RATEL:-1}
DGRAPH_BUILD_AMD64=${DGRAPH_BUILD_AMD64:-1}
DGRAPH_BUILD_ARM64=${DGRAPH_BUILD_ARM64:-0}

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

check_command_exists nvm
check_command_exists npm

# TODO Check if ports 8000, 9080, or 6080 are bound already and error out early.

check_command_exists strip
check_command_exists make
check_command_exists gcc
check_command_exists go
check_command_exists docker
check_command_exists docker-compose
check_command_exists protoc
check_command_exists shasum
check_command_exists tar
check_command_exists zip

nvm install --lts=Fermium # Fermium is Node v14 LTS

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
GOVERSION=${GOVERSION:-"1.17.3"}

TAG=$1

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
codenameKey="github.com/dgraph-io/dgraph/x.dgraphCodename"
branch="github.com/dgraph-io/dgraph/x.gitBranch"
commitSHA1="github.com/dgraph-io/dgraph/x.lastCommitSHA"
commitTime="github.com/dgraph-io/dgraph/x.lastCommitTime"
jemallocXgoFlags=

# Get xgo and docker image
if [[ $GOVERSION =~ ^1\.16.* ]] || [[ $GOVERSION =~ ^1\.17.* ]]; then
  # Build xgo docker image with 'go env -w GO111MODULE=auto' to support 1.16.x
  docker build -f release/xgo.Dockerfile -t dgraph/xgo:go-${GOVERSION} --build-arg GOVERSION=${GOVERSION} .
  # Instruct xgo to use alternative image
  export DGRAPH_BUILD_XGO_IMAGE="-image dgraph/xgo:go-${GOVERSION}"
fi
go install src.techknowlogick.com/xgo
mkdir -p ~/.xgo-cache


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
  codename="$(awk '/^BUILD_CODENAME/ { print $NF }' ./dgraph/Makefile)"
  gitBranch=$(git rev-parse --abbrev-ref HEAD)
  lastCommitTime=$(git log -1 --format=%ci)
  release_version=$(git describe --always --tags)
popd

# The Docker tag should not contain a slash e.g. feature/issue1234
# The initial slash is taken from the repository name dgraph/dgraph:tag
DOCKER_TAG=${2:-$release_version}

# Build the JS lambda server.
pushd $basedir/dgraph/lambda
  make build
popd

# Regenerate protos. Should not be different from what's checked in.
pushd $basedir/dgraph/protos
  # We need to fetch the modules to get the correct proto files. e.g., for
  # badger and dgo
  go get -d -v ../dgraph

  make regenerate
  if [[ "$(git status --porcelain .)" ]]; then
      echo >&2 "Generated protos different in release."
      exit 1
  fi
popd

# Clone Badger repo.
pushd $basedir
  git clone https://github.com/dgraph-io/badger.git
  # Check out badger version specific to the Dgraph release.
  cd ./badger
  ref="$(grep github.com/dgraph-io/badger/v3 $basedir/dgraph/go.mod | grep -v replace |  awk '{ print $2 }')"
  commitish="$(echo "$ref" | awk -F- '{ print $NF }')"
  git checkout "$commitish"
popd

if [[ $DGRAPH_BUILD_RATEL =~ 1|true ]]; then
  # Clone ratel repo.
  pushd $basedir
    git clone https://github.com/dgraph-io/ratel.git
  popd

  # build ratel client
  pushd $basedir/ratel
    (export GO111MODULE=off; ./scripts/build.prod.sh)
    ./scripts/test.sh
  popd
fi

build_windows() {
  # Build Windows.
  pushd $basedir/dgraph/dgraph
    xgo -x -go="go-$GOVERSION" --targets=windows/$GOARCH $DGRAPH_BUILD_XGO_IMAGE -buildmode=exe -ldflags \
        "-X $release=$release_version -X $codenameKey=$codename -X $branch=$gitBranch -X $commitSHA1=$lastCommitSHA1 -X '$commitTime=$lastCommitTime'" .
    mkdir -p $TMP/$GOARCH/windows
    mv dgraph-windows-4.0-$GOARCH.exe $TMP/windows/$GOARCH/dgraph.exe
  popd

  pushd $basedir/badger/badger
    xgo -x -go="go-$GOVERSION" --targets=windows/$GOARCH $DGRAPH_BUILD_XGO_IMAGE -buildmode=exe .
    mv badger-windows-4.0-$GOARCH.exe $TMP/windows/$GOARCH/badger.exe
  popd

  if [[ $DGRAPH_BUILD_RATEL =~ 1|true ]]; then
    pushd $basedir/ratel
      xgo -x -go="go-$GOVERSION" --targets=windows/$GOARCH $DGRAPH_BUILD_XGO_IMAGE -ldflags "-X $ratel_release=$release_version"  -buildmode=exe .
      mv ratel-windows-4.0-$GOARCH.exe $TMP/windows/$GOARCH/dgraph-ratel.exe
    popd
  fi
}

build_darwin() {
  # Build Darwin.
  pushd $basedir/dgraph/dgraph
    xgo -x -go="go-$GOVERSION" --targets=darwin-10.9/$GOARCH $DGRAPH_BUILD_XGO_IMAGE -ldflags \
    "-X $release=$release_version -X $codenameKey=$codename -X $branch=$gitBranch -X $commitSHA1=$lastCommitSHA1 -X '$commitTime=$lastCommitTime'" .
    mkdir -p $TMP/darwin/$GOARCH
    mv dgraph-darwin-10.9-$GOARCH $TMP/darwin/$GOARCH/dgraph
  popd

  pushd $basedir/badger/badger
    xgo -x -go="go-$GOVERSION" --targets=darwin-10.9/$GOARCH $DGRAPH_BUILD_XGO_IMAGE .
    mv badger-darwin-10.9-$GOARCH $TMP/darwin/$GOARCH/badger
  popd

  if [[ $DGRAPH_BUILD_RATEL =~ 1|true ]]; then
    pushd $basedir/ratel
      xgo -x -go="go-$GOVERSION" --targets=darwin-10.9/$GOARCH $DGRAPH_BUILD_XGO_IMAGE -ldflags "-X $ratel_release=$release_version" .
      mv ratel-darwin-10.9-$GOARCH $TMP/darwin/$GOARCH/dgraph-ratel
    popd
  fi
}

build_linux() {
  # Build Linux.
  pushd $basedir/dgraph/dgraph
    xgo -x -v -go="go-$GOVERSION" --targets=linux/$GOARCH $DGRAPH_BUILD_XGO_IMAGE -ldflags \
       "-X $release=$release_version -X $codenameKey=$codename -X $branch=$gitBranch -X $commitSHA1=$lastCommitSHA1 -X '$commitTime=$lastCommitTime'" --tags=jemalloc -deps=https://github.com/jemalloc/jemalloc/releases/download/5.2.1/jemalloc-5.2.1.tar.bz2  --depsargs='--with-jemalloc-prefix=je_ --with-malloc-conf=background_thread:true,metadata_thp:auto --enable-prof' .
    strip -x dgraph-linux-$GOARCH
    mkdir -p $TMP/linux/$GOARCH
    mv dgraph-linux-$GOARCH $TMP/linux/$GOARCH/dgraph
  popd

  pushd $basedir/badger/badger
    xgo -x -v -go="go-$GOVERSION" --targets=linux/$GOARCH $DGRAPH_BUILD_XGO_IMAGE --tags=jemalloc -deps=https://github.com/jemalloc/jemalloc/releases/download/5.2.1/jemalloc-5.2.1.tar.bz2  --depsargs='--with-jemalloc-prefix=je_ --with-malloc-conf=background_thread:true,metadata_thp:auto --enable-prof' .
    strip -x badger-linux-$GOARCH
    mv badger-linux-$GOARCH $TMP/linux/$GOARCH/badger
  popd

  if [[ $DGRAPH_BUILD_RATEL =~ 1|true ]]; then
    pushd $basedir/ratel
      xgo -x -v -go="go-$GOVERSION" --targets=linux/$GOARCH $DGRAPH_BUILD_XGO_IMAGE -ldflags "-X $ratel_release=$release_version"  .
      strip -x ratel-linux-$GOARCH
      mv ratel-linux-$GOARCH $TMP/linux/dgraph-ratel
    popd
  fi
}

createSum () {
  os=$1
  echo "Creating checksum for $os"
  if [[ "$os" != "windows" ]]; then
    pushd $TMP/$os/$GOARCH
      csum=$(shasum -a 256 dgraph | awk '{print $1}')
      echo $csum /usr/local/bin/dgraph >> ../dgraph-checksum-$os-$GOARCH.sha256
      csum=$(shasum -a 256 dgraph-ratel | awk '{print $1}')
      echo $csum /usr/local/bin/dgraph-ratel >> ../dgraph-checksum-$os-$GOARCH.sha256
    popd
  else
    pushd $TMP/$os/$GOARCH
      csum=$(shasum -a 256 dgraph.exe | awk '{print $1}')
      echo $csum dgraph.exe >> ../dgraph-checksum-$os-$GOARCH.sha256
      csum=$(shasum -a 256 dgraph-ratel.exe | awk '{print $1}')
      echo $csum dgraph-ratel.exe >> ../dgraph-checksum-$os-$GOARCH.sha256
    popd
  fi
}

## TODO: Add arm64 buildkit support once xgo works for arm64
build_docker_image() {
  if [[ "$GOARCH" == "amd64" ]]; then
    # Create Dgraph Docker image.
    # edit Dockerfile to point to binaries 
    sed "s/^ADD linux/ADD linux\/$GOARCH/" $basedir/dgraph/contrib/Dockerfile > $TMP/Dockerfile
    pushd $TMP
      # Get a fresh ubuntu:latest image each time
      # Don't rely on whatever "latest" version
      # happens to be on the machine.
      docker pull ubuntu:latest

      docker build -t dgraph/dgraph:$DOCKER_TAG .
    popd
    rm $TMP/Dockerfile

    # Create Dgraph standalone Docker image.
    pushd $basedir/dgraph/contrib/standalone
      make DGRAPH_VERSION=$DOCKER_TAG
    popd
  fi
}

# Create the tar and delete the binaries.
createTar () {
  os=$1
  echo "Creating tar for $os"
  pushd $TMP/$os/$GOARCH
    tar -zcvf ../dgraph-$os-$GOARCH.tar.gz *
  popd
  rm -Rf $TMP/$os/$GOARCH
}

# Create the zip and delete the binaries.
createZip () {
  os=$1
  echo "Creating zip for $os"
  pushd $TMP/$os/$GOARCH
    zip -r ../dgraph-$os-$GOARCH.zip *
  popd
  rm -Rf $TMP/$os/$GOARCH
}

build_artifacts() {
  # Build Binaries
  [[ $DGRAPH_BUILD_WINDOWS =~ 1|true ]] && build_windows
  [[ $DGRAPH_BUILD_MAC =~ 1|true ]] && build_darwin
  build_linux

  # Build Checksums
  createSum linux
  [[ $DGRAPH_BUILD_MAC =~ 1|true ]] && createSum darwin
  [[ $DGRAPH_BUILD_WINDOWS =~ 1|true ]] && createSum windows

  # Build Docker images
  build_docker_image

  # Build Archives
  createTar linux
  [[ $DGRAPH_BUILD_WINDOWS =~ 1|true ]] && createZip windows
  [[ $DGRAPH_BUILD_MAC =~ 1|true ]] && createTar darwin

  if [[ "$GOARCH" == "amd64" ]]; then
    echo "Release $TAG is ready."
    docker run dgraph/dgraph:$DOCKER_TAG dgraph
  fi
  ls -alh $TMP
}

if [[ $DGRAPH_BUILD_AMD64 =~ 1|true ]]; then
  export GOARCH=amd64
  build_artifacts
fi

## Currently arm64 xgo fails for dgraph and badger
## * https://github.com/techknowlogick/xgo/issues/105
if [[ $DGRAPH_BUILD_ARM64 =~ 1|true ]]; then
  export GOARCH=arm64
  build_artifacts
fi

set +o xtrace
echo "To release:"
if git show-ref -q --verify "refs/tags/$TAG"; then
    echo
    echo "Push the git tag"
    echo "  git push origin $TAG"
fi
echo
echo "Push the Docker tag:"
echo "  docker push dgraph/dgraph:$DOCKER_TAG"
echo "  docker push dgraph/standalone:$DOCKER_TAG"
echo
echo "If this should be the latest release, then tag"
echo "the image as latest too."
echo "  docker tag dgraph/dgraph:$DOCKER_TAG dgraph/dgraph:latest"
echo "  docker tag dgraph/standalone:$DOCKER_TAG dgraph/standalone:latest"
echo "  docker push dgraph/dgraph:latest"
echo "  docker push dgraph/standalone:latest"
