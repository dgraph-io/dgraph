#!/bin/bash

SRC="$( cd -P "$( dirname "${BASH_SOURCE[0]}" )" && pwd )/.."

BUILD=$1
# If build variable is empty then we set it.
if [ -z "$1" ]; then
  BUILD=$SRC/build
fi

ROCKSDBDIR=$BUILD/rocksdb-5.1.4

set -e

pushd $BUILD &> /dev/null

if [ ! -f "goldendata.rdf.gz" ]; then
  wget https://github.com/dgraph-io/benchmarks/raw/master/data/goldendata.rdf.gz
fi

# log file size.
ls -la goldendata.rdf.gz

benchmark=$(pwd)
popd &> /dev/null

# build flags needed for rocksdb
export CGO_CPPFLAGS="-I${ROCKSDBDIR}/include"
export CGO_LDFLAGS="-L${ROCKSDBDIR}"
export LD_LIBRARY_PATH="${ROCKSDBDIR}:${LD_LIBRARY_PATH}"

pushd cmd/dgraph &> /dev/null
go build .
./dgraph -gentlecommit 1.0 &
popd &> /dev/null

sleep 15

#Set Schema
curl -X POST  -d 'mutation {
  schema {
	  name: string @index
	  initial_release_date: date @index
	}
}' "http://localhost:8080/query"

pushd cmd/dgraphloader &> /dev/null
go build .
./dgraphloader -r $benchmark/goldendata.rdf.gz
popd &> /dev/null

# Lets wait for stuff to be committed to RocksDB.
sleep 20

pushd $GOPATH/src/github.com/dgraph-io/dgraph/contrib/indextest &> /dev/null

function run_index_test {
	X=$1
	GREPFOR=$2
	ANS=$3
    N=`curl localhost:8080/query -XPOST -d @${X}.in 2> /dev/null | python -m json.tool | grep $GREPFOR | wc -l`
	if [[ ! "$N" -eq "$ANS" ]]; then
	  echo "Index test failed: ${X}  Expected: $ANS  Got: $N"
	  exit 1
	fi
}
run_index_test basic name 138676
run_index_test allof_the name 25431
run_index_test allof_the_a name 367
run_index_test allof_the_first name 4383
run_index_test releasedate release_date 137858
run_index_test releasedate_sort release_date 137858
run_index_test releasedate_sort_first_offset release_date 2315
run_index_test releasedate_geq release_date 60991
run_index_test gen_anyof_good_bad name 1104

popd &> /dev/null

curl localhost:8080/admin/shutdown
