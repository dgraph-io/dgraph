#!/bin/bash

SRC="$( cd -P "$( dirname "${BASH_SOURCE[0]}" )" && pwd )/.."

BUILD=$1
# If build variable is empty then we set it.
if [ -z "$1" ]; then
	  BUILD=$SRC/build
fi

ROCKSDBDIR=$BUILD/rocksdb-${ROCKSDBVER}

# build flags needed for rocksdb
export CGO_CFLAGS="-I${ROCKSDBDIR}/include"
export CGO_LDFLAGS="-L${ROCKSDBDIR}"
export LD_LIBRARY_PATH="${ROCKSDBDIR}:${LD_LIBRARY_PATH}"

set -e

pushd $SRC &> /dev/null

# Get git-lfs and benchmark data.
wget https://github.com/github/git-lfs/releases/download/v1.3.1/git-lfs-linux-amd64-1.3.1.tar.gz
tar -xzf git-lfs-linux-amd64-1.3.1.tar.gz
pushd git-lfs-1.3.1 &> /dev/null
sudo /bin/bash ./install.sh
popd &> /dev/null

git clone https://github.com/dgraph-io/benchmarks.git
benchmark=$(pwd)/data

popd &> /dev/null

# Run the assigner.
dgraphcmd=$GOPATH/src/dgraph-io/dgraph/cmd/

pushd $dgraphcmd/dgraphassigner &> /dev/null

go build .

./dgraphassigner --numInstances 1 --instanceIdx 0 --rdfgzips $benchmark/rdf-films.gz,$benchmark/names.gz --uids ~/dgraph/u

popd &> /dev/null
