#!/bin/bash

export ROCKSDBVER="4.2"
ROCKSDBURL="https://github.com/facebook/rocksdb/archive/v${ROCKSDBVER}.tar.gz"
ROCKSDBFILE="rocksdb-${ROCKSDBVER}.tar.gz"
ROCKSDBDIR=rocksdb-${ROCKSDBVER}
ROCKSDBLIB=librocksdb.so.${ROCKSDBVER}

# Gets the path of the directory from which the script is executed.
SRC="$( cd -P "$( dirname "${BASH_SOURCE[0]}" )" && pwd )/.."

BUILD=$1
if [ -z "$1" ]; then
  BUILD=$SRC/build
fi

[ -d $BUILD ] || mkdir -p $BUILD

set -e

# Push build directory to stack, checkout and don't display output.
pushd $BUILD &> /dev/null

# download
if [ ! -f $ROCKSDBFILE ]; then
  wget -O $ROCKSDBFILE $ROCKSDBURL
fi

# extract
if [ ! -d $ROCKSDBDIR ]; then
  tar -zxf $ROCKSDBFILE
fi

# configure, build
if [ ! -e $ROCKSDBDIR/${ROCKSDBLIB} ]; then
  cd $ROCKSDBDIR
  make shared_lib
fi

# check back out to the directory you were in earlier.
popd &> /dev/null
