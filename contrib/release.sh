#!/bin/bash

# Script to do Dgraph release. This script would output the built binaries in
# $TMP.  This script should NOT be responsible for doing any testing, or
# uploading to any server.  The sole task of this script is to build the
# binaries and prepare them such that any human or script can then pick these up
# and use them as they deem fit.

# Output colors
RED='\033[91;1m'
RESET='\033[0m'

# Don't use standard GOPATH. Create a new one.
GOPATH="/tmp/go"
rm -Rf $GOPATH
mkdir $GOPATH
# Necessary to pick up Gobin binaries like protoc-gen-gofast
PATH="$GOPATH/bin:$PATH"

# The Go version used for release builds must match this version.
GOVERSION="1.12.5"

TAG=$1
# The Docker tag should not contain a slash e.g. feature/issue1234
# The initial slash is taken from the repository name dgraph/dgraph:tag
DTAG=$(echo "$TAG" | tr '/' '-')


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

# Check for existence of strip tool.
type strip
type shasum

ratel_release="github.com/dgraph-io/ratel/server.ratelVersion"
release="github.com/dgraph-io/dgraph/x.dgraphVersion"
branch="github.com/dgraph-io/dgraph/x.gitBranch"
commitSHA1="github.com/dgraph-io/dgraph/x.lastCommitSHA"
commitTime="github.com/dgraph-io/dgraph/x.lastCommitTime"

echo "Using Go version"
go version
if [[ ! "$(go version)" =~ $GOVERSION ]]; then
   echo -e "${RED}Go version is NOT expected. Should be $GOVERSION.${RESET}"
   exit 1
fi

go get -u github.com/jteeuwen/go-bindata/...
go get -d -u golang.org/x/net/context
go get -d google.golang.org/grpc
go get -u github.com/prometheus/client_golang/prometheus
go get -u github.com/dgraph-io/dgo
# go get github.com/stretchr/testify/require
go get -u github.com/dgraph-io/badger
go get -u github.com/golang/protobuf/protoc-gen-go
go get -u github.com/gogo/protobuf/protoc-gen-gofast

pushd $GOPATH/src/google.golang.org/grpc
  git checkout v1.13.0
popd

basedir=$GOPATH/src/github.com/dgraph-io
# Clone Dgraph repo.
pushd $basedir
  git clone https://github.com/dgraph-io/dgraph.git
popd

pushd $basedir/dgraph
  git pull
  git checkout $TAG
  # HEAD here points to whatever is checked out.
  lastCommitSHA1=$(git rev-parse --short HEAD)
  gitBranch=$(git rev-parse --abbrev-ref HEAD)
  lastCommitTime=$(git log -1 --format=%ci)
  release_version=$TAG
popd

# Regenerate protos. Should not be different from what's checked in.
pushd $basedir/dgraph/protos
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
  git pull
  source ~/.nvm/nvm.sh
  nvm install --lts
  ./scripts/build.prod.sh
popd

# Build Windows.
pushd $basedir/dgraph/dgraph
  env GOOS=windows GOARCH=amd64 go get -v -d .
  env GOOS=windows GOARCH=amd64 go build -v -o dgraph-windows-amd64.exe -ldflags \
      "-X $release=$release_version -X $branch=$gitBranch -X $commitSHA1=$lastCommitSHA1 -X '$commitTime=$lastCommitTime'" .
  mkdir $TMP/windows
  mv dgraph-windows-amd64.exe $TMP/windows/dgraph.exe
popd

pushd $basedir/badger/badger
  env GOOS=windows GOARCH=amd64 go get -v -d .
  env GOOS=windows GOARCH=amd64 go build -v -o badger-windows-amd64.exe .
  mv badger-windows-amd64.exe $TMP/windows/badger.exe
popd

pushd $basedir/ratel
  env GOOS=windows GOARCH=amd64 go get -v -d .
  env GOOS=windows GOARCH=amd64 go build -v -o ratel-windows-amd64.exe -ldflags "-X $ratel_release=$release_version" .
  mv ratel-windows-amd64.exe $TMP/windows/dgraph-ratel.exe
popd

# Build Darwin.
pushd $basedir/dgraph/dgraph
  env GOOS=darwin GOARCH=amd64 go get -v -d .
  env GOOS=darwin GOARCH=amd64 go build -v -o dgraph-darwin-amd64 -ldflags \
      "-X $release=$release_version -X $branch=$gitBranch -X $commitSHA1=$lastCommitSHA1 -X '$commitTime=$lastCommitTime'" .
  mkdir $TMP/darwin
  mv dgraph-darwin-amd64 $TMP/darwin/dgraph
popd

pushd $basedir/badger/badger
  env GOOS=darwin GOARCH=amd64 go get -v -d .
  env GOOS=darwin GOARCH=amd64 go build -v -o badger-darwin-amd64 .
  mv badger-darwin-amd64 $TMP/darwin/badger
popd

pushd $basedir/ratel
  env GOOS=darwin GOARCH=amd64 go get -v -d .
  env GOOS=darwin GOARCH=amd64 go build -v -o ratel-darwin-amd64 -v -ldflags "-X $ratel_release=$release_version" .
  mv ratel-darwin-amd64 $TMP/darwin/dgraph-ratel
popd

# Build Linux.
pushd $basedir/dgraph/dgraph
  env GOOS=linux GOARCH=amd64 go get -v -d .
  env GOOS=linux GOARCH=amd64 go build -v -o dgraph-linux-amd64 -ldflags \
      "-X $release=$release_version -X $branch=$gitBranch -X $commitSHA1=$lastCommitSHA1 -X '$commitTime=$lastCommitTime'" .
  strip -x dgraph-linux-amd64
  mkdir $TMP/linux
  mv dgraph-linux-amd64 $TMP/linux/dgraph
popd

pushd $basedir/badger/badger
  env GOOS=linux GOARCH=amd64 go get -v -d .
  env GOOS=linux GOARCH=amd64 go build -v -o badger-linux-amd64 .
  strip -x badger-linux-amd64
  mv badger-linux-amd64 $TMP/linux/badger
popd

pushd $basedir/ratel
  env GOOS=linux GOARCH=amd64 go get -v -d .
  env GOOS=linux GOARCH=amd64 go build -v -o ratel-linux-amd64 -ldflags "-X $ratel_release=$release_version" .
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

# Create Docker image.
cp $basedir/dgraph/contrib/Dockerfile $TMP
pushd $TMP
  docker build -t dgraph/dgraph:$DTAG .
popd
rm $TMP/Dockerfile

# Create the tars and delete the binaries.
createTar () {
  os=$1
  echo "Creating tar for $os"
  pushd $TMP/$os
    tar -zcvf ../dgraph-$os-amd64.tar.gz *
  popd
  rm -Rf $TMP/$os
}

createTar windows
createTar darwin
createTar linux

echo "Release $TAG is ready."
docker run -it dgraph/dgraph:$DTAG dgraph
ls -alh $TMP
