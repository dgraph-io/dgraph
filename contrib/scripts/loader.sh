#!/bin/bash

contrib=$GOPATH/src/github.com/dgraph-io/dgraph/contrib
source $contrib/scripts/functions.sh

SRC="$( cd -P "$( dirname "${BASH_SOURCE[0]}" )" && pwd )/.."

BUILD=$1
# If build variable is empty then we set it.
if [ -z "$1" ]; then
  BUILD=$SRC/build
fi

mkdir -p $BUILD

set -e

pushd $BUILD &> /dev/null
if [ ! -f "goldendata.rdf.gz" ]; then
  cp $GOPATH/src/github.com/dgraph-io/dgraph/systest/data/goldendata.rdf.gz .
fi

# log file size.
ls -la goldendata.rdf.gz

benchmark=$(pwd)
popd &> /dev/null

startZero
# Start Dgraph
start

#Set Schema
curl -X PUT  -d '
    name: string @index(term) .
    initial_release_date: datetime @index(year) .
' "http://localhost:8081/alter"

echo -e "\nRunning dgraph live."
# Delete client directory to clear xidmap.

rm -rf $BUILD/xiddir
pushd dgraph &> /dev/null
./dgraph live -r $benchmark/goldendata.rdf.gz -d "127.0.0.1:9081,127.0.0.1:9082" -z "127.0.0.1:5080" -c 100 -b 1000 -x $BUILD/xiddir
popd &> /dev/null

# Restart Dgraph so that we are sure that index keys are persisted.
quit 0

startZero
start

$contrib/scripts/goldendata-queries.sh

quit 0

echo -e "Trying to restart Dgraph and match export count"
startZero
start

echo -e "Trying to export data."
rm -rf export/*
curl http://localhost:8081/admin/export
echo -e "\nExport done."

pushd dgraph &> /dev/null
# This is count of RDF's in goldendata.rdf.gz
dataCount="1120879"
# Concat exported files to get total count.
cat $(ls -t export/dgraph-1-* | head -1) $(ls -t export/dgraph-2-* | head -1) > export/dgraph-export.rdf.gz
exportCount=$(zcat export/dgraph-export.rdf.gz | wc -l)
popd &> /dev/null

if [[ ! "$exportCount" -eq "$dataCount" ]]; then
  echo "Export test failed. Expected: $dataCount Got: $exportCount"
  quit 1
else
  echo "Export count matches"
fi

quit 0
