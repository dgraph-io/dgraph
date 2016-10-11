#!/bin/bash

ICUURL="http://download.icu-project.org/files/icu4c/57.1/icu4c-57_1-src.tgz"
ICUFILE="icu4c-57_1-src.tgz"

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

echo $BUILD

# download
if [ ! -f $ICUFILE ]; then
  wget -O $ICUFILE $ICUURL
fi

# extract
if [ ! -d icu ]; then
  tar -zxf $ICUFILE  # Should create a directory "icu".
	mkdir -p icu/build
fi

# configure, build
if [ ! -e icu/build/lib/libicuuc.so ]; then
  cd icu/source
	./configure --prefix=$BUILD/icu/build --disable-renaming
  make
  make install
fi

# check back out to the directory you were in earlier.
popd &> /dev/null
