#!/bin/bash

ROCKSDBVER="4.2"
ROCKSDBURL="https://github.com/facebook/rocksdb/archive/v${ROCKSDBVER}.tar.gz"
ROCKSDBFILE="rocksdb-${ROCKSDBVER}.tar.gz"
ROCKSDBDIR=rocksdb-${ROCKSDBVER}
ROCKSDBLIB=librocksdb.so.${ROCKSDBVER}

SRC="$( cd -P "$( dirname "${BASH_SOURCE[0]}" )" && pwd )/.."

BUILD=$1
if [ -z "$1" ]; then
  BUILD=$SRC/build
fi

[ -d $BUILD ] || mkdir -p $BUILD

set -e

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

popd &> /dev/null
