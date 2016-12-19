#!/bin/bash

SRC="$( cd -P "$( dirname "${BASH_SOURCE[0]}" )" && pwd )/.."
TMP=$(mktemp -p /tmp dgraph-coverage-XXXXX.txt)

BUILD=$1
# If build variable is empty then we set it.
if [ -z "$1" ]; then
  BUILD=$SRC/build
fi

OUT=$2
if [ -z "$OUT" ]; then
  OUT=$SRC/coverage.out
fi
rm -f $OUT

ROCKSDBDIR=$BUILD/rocksdb-4.11.2
ICUDIR=$BUILD/icu/build

# build flags needed for rocksdb
export CGO_CPPFLAGS="-I${ROCKSDBDIR}/include -I${ICUDIR}/include"
export CGO_LDFLAGS="-L${ROCKSDBDIR} -L${ICUDIR}/lib"
export LD_LIBRARY_PATH="${ICUDIR}/lib:${ROCKSDBDIR}:${LD_LIBRARY_PATH}"

set -e

go get golang.org/x/net/context
go get google.golang.org/grpc/...
pushd $SRC &> /dev/null

# create coverage output
echo 'mode: atomic' > $OUT
for PKG in $(go list ./...|grep -v '/vendor/' | grep -v '/contrib/'); do
  echo "TESTING: $PKG"
  go test -v -covermode=atomic -coverprofile=$TMP $PKG
  tail -n +2 $TMP >> $OUT
done

# open in browser if not in a build environment
if [ ! -z "$DISPLAY" ]; then
  go tool cover -html=$OUT
fi

popd &> /dev/null
