#!/bin/bash

SRC="$( cd -P "$( dirname "${BASH_SOURCE[0]}" )" && pwd )/.."

BUILD=$1
# If build variable is empty then we set it.
if [ -z "$1" ]; then
	  BUILD=$SRC/build
fi

ROCKSDBDIR=$BUILD/rocksdb-4.2

set -e

pushd $BUILD &> /dev/null

gitlfsfile="git-lfs-1.3.1"
rm -rf $gitlfsfile
if [ ! -d $gitlfsfile ]; then
  # Get git-lfs and benchmark data.
  wget https://github.com/github/git-lfs/releases/download/v1.3.1/git-lfs-linux-amd64-1.3.1.tar.gz
  tar -xzf git-lfs-linux-amd64-1.3.1.tar.gz
  pushd git-lfs-1.3.1 &> /dev/null
  sudo /bin/bash ./install.sh
  popd &> /dev/null
fi

rm -rf benchmarks
if [ ! -f "benchmarks/data/actor-director.gz" ]; then
	git clone https://github.com/dgraph-io/benchmarks.git
fi
benchmark=$(pwd)/benchmarks/data
popd &> /dev/null
# We are back in the Dgraph repo.

# build flags needed for rocksdb
export CGO_CFLAGS="-I${ROCKSDBDIR}/include"
export CGO_LDFLAGS="-L${ROCKSDBDIR}"
export LD_LIBRARY_PATH="${ROCKSDBDIR}:${LD_LIBRARY_PATH}"

pushd cmd/dgraphassigner &> /dev/null
go build .

mkdir -p ~/dgraph/u
./dgraphassigner --numInstances 1 --instanceIdx 0 --rdfgzips $benchmark/actor-director.gz --uids ~/dgraph/u

popd &> /dev/null
