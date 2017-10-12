#!/bin/bash

source ./contrib/functions.sh

SRC="$( cd -P "$( dirname "${BASH_SOURCE[0]}" )" && pwd )/.."

BUILD=$1
# If build variable is empty then we set it.
if [ -z "$1" ]; then
  BUILD=$SRC/build
fi

set -e

pushd $BUILD &> /dev/null

if [ ! -f "goldendata.rdf.gz" ]; then
  echo -e "\nDownloading goldendata."
  wget -q https://github.com/dgraph-io/benchmarks/raw/master/data/goldendata.rdf.gz
fi

# log file size.
ls -la goldendata.rdf.gz

benchmark=$(pwd)
popd &> /dev/null

pushd cmd/dgraphzero &> /dev/null
echo -e "\nBuilding and running Dgraph Zero."
go build .

startZero
popd &> /dev/null

pushd cmd/dgraph &> /dev/null
echo -e "\nBuilding and running Dgraph."
go build .

./dgraph --version
if [ $? -eq 0 ]; then
  echo -e "dgraph --version succeeded.\n"
else
  echo "dgraph --version command failed."
fi

# Start Dgraph
start
popd &> /dev/null

#Set Schema
curl -X POST  -d 'mutation {
  schema {
    name: string @index(term) .
    xid: string @index(exact) .
    initial_release_date: datetime @index(year) .
  }
}' "http://localhost:8080/query"

res=$(curl -X POST  -d 'schema {}' "http://localhost:8080/query")
expected='{"data":{"schema":[{"predicate":"_predicate_","type":"string","list":true},{"predicate":"initial_release_date","type":"datetime","index":true,"tokenizer":["year"]},{"predicate":"name","type":"string","index":true,"tokenizer":["term"]},{"predicate":"xid","type":"string","index":true,"tokenizer":["exact"]}]}}'
echo -e "Response $res"
if [[ $TRAVIS_OS_NAME ==  "linux" ]] && [[ "$res" != "$expected" ]]; then
  echo "Schema comparison failed."
  quit 1
else
  echo "Schema comparison successful."
fi

echo -e "\nBuilding and running dgraph-live-loader."
pushd cmd/dgraph-live-loader &> /dev/null
# Delete client directory to clear checkpoints.
rm -rf c
go build .
./dgraph-live-loader -r $benchmark/goldendata.rdf.gz -x true -d "localhost:8080,localhost:8082"
popd &> /dev/null


# Restart Dgraph so that we are sure that index keys are persisted.
quit 0
# Wait for a clean shutdown.
echo -e "\nClean Shutdown Done"
pushd cmd/dgraphzero &> /dev/null
startZero
popd &> /dev/null

pushd cmd/dgraph &> /dev/null
start
popd &> /dev/null

./contrib/goldendata-queries.sh

echo -e "\nShutting down Dgraph"
quit 0

echo -e "\nTrying to restart Dgraph and match export count"
pushd cmd/dgraphzero &> /dev/null
startZero
popd &> /dev/null

pushd cmd/dgraph &> /dev/null
start
echo -e "\nTrying to export data."
rm -rf export/*
curl http://localhost:8080/admin/export
echo -e "\nExport done."

# This is count of RDF's in goldendata.rdf.gz + xids because we ran dgraph-live-loader with -xid flag.
dataCount="1475250"
# Concat exported files to get total count.
cat $(ls -t export/dgraph-1-* | head -1) $(ls -t export/dgraph-2-* | head -1) > export/dgraph-export.rdf.gz
if [[ $TRAVIS_OS_NAME == "osx" ]]; then
  exportCount=$(zcat < export/dgraph-export.rdf.gz | wc -l)
else
  exportCount=$(zcat export/dgraph-export.rdf.gz | wc -l)
fi


if [[ ! "$exportCount" -eq "$dataCount" ]]; then
  echo "Export test failed. Expected: $dataCount Got: $exportCount"
  quit 1
fi

echo -e "All tests ran fine. Exiting"
quit 0
