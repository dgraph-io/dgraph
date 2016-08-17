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

echo "before src" $(pwd)
pushd $BUILD &> /dev/null
echo "after src" $BUILD $(pwd)

# Get git-lfs and benchmark data.
wget https://github.com/github/git-lfs/releases/download/v1.3.1/git-lfs-linux-amd64-1.3.1.tar.gz
tar -xzf git-lfs-linux-amd64-1.3.1.tar.gz
pushd git-lfs-1.3.1 &> /dev/null
echo "after git-lfs" $(pwd)
sudo /bin/bash ./install.sh
popd &> /dev/null
echo "after pop git-lfs" $(pwd)

git clone https://github.com/dgraph-io/benchmarks.git
benchmark=$(pwd)/benchmarks/data
echo $(pwd)
echo $benchmark
popd &> /dev/null
echo $(pwd)

# Run the assigner.
dgraphcmd=$GOPATH/src/dgraph-io/dgraph/cmd

pushd $dgraphcmd/dgraphassigner &> /dev/null
echo $(pwd)

go build .

./dgraphassigner --numInstances 1 --instanceIdx 0 --rdfgzips $benchmark/rdf-films.gz,$benchmark/names.gz --uids ~/dgraph/u

popd &> /dev/null
