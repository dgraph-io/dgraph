#!/bin/bash

SRC="$( cd -P "$( dirname "${BASH_SOURCE[0]}" )" && pwd )/.."
TMP=$(mktemp -p /tmp dgraph-coverage-XXXXX.txt)

ROCKSDBDIR=$SRC/build/rocksdb-4.2

OUT=$1
if [ -z "$OUT" ]; then
  OUT=$SRC/coverage.out
fi
rm -f $OUT

set -e

pushd $SRC &> /dev/null

# build flags needed for rocksdb
export CGO_CFLAGS="-I${ROCKSDBDIR}/include"
export CGO_LDFLAGS="-L${ROCKSDBDIR}"
export LD_LIBRARY_PATH="${ROCKSDBDIR}:${LD_LIBRARY_PATH}"

# create coverage output
echo 'mode: atomic' > $OUT
for PKG in $(go list ./...|grep -v '/vendor/'); do
  echo "TESTING: $PKG"
  go test -v -covermode=atomic -coverprofile=$TMP $PKG
  tail -n +2 $TMP >> $OUT
done

# open in browser if not in a build environment
if [ ! -z "$DISPLAY" ]; then
  go tool cover -html=$OUT
fi

popd &> /dev/null
