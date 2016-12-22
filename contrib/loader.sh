#!/bin/bash

SRC="$( cd -P "$( dirname "${BASH_SOURCE[0]}" )" && pwd )/.."

BUILD=$1
# If build variable is empty then we set it.
if [ -z "$1" ]; then
  BUILD=$SRC/build
fi

ROCKSDBDIR=$BUILD/rocksdb-4.11.2
ICUDIR=$BUILD/icu/build

set -e

pushd $BUILD &> /dev/null

gitlfsfile="git-lfs-1.3.1"
if [ ! -d $gitlfsfile ]; then
  # Get git-lfs and benchmark data.
  wget https://github.com/github/git-lfs/releases/download/v1.3.1/git-lfs-linux-amd64-1.3.1.tar.gz
  tar -xzf git-lfs-linux-amd64-1.3.1.tar.gz
  pushd git-lfs-1.3.1 &> /dev/null
  sudo /bin/bash ./install.sh
  popd &> /dev/null
fi

if [ ! -f "benchmarks/data/goldendata.rdf.gz" ]; then
	git lfs init
    git clone https://github.com/dgraph-io/benchmarks.git
fi
benchmark=$(pwd)/benchmarks/data
popd &> /dev/null

# build flags needed for rocksdb
export CGO_CPPFLAGS="-I${ROCKSDBDIR}/include -I${ICUDIR}/include"
export CGO_LDFLAGS="-L${ROCKSDBDIR} -L${ICUDIR}/lib"
export LD_LIBRARY_PATH="${ICUDIR}/lib:${ROCKSDBDIR}:${LD_LIBRARY_PATH}"

pushd cmd/dgraph &> /dev/null
go build .
./dgraph -gentlecommit 0.5 &
popd &> /dev/null

sleep 5

pushd cmd/dgraphloader &> /dev/null
go build .
./dgraphloader -r $benchmark/goldendata.rdf.gz
popd &> /dev/null

killall dgraph
