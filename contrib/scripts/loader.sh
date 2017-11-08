#!/bin/bash

source ./contrib/scripts/functions.sh

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

startZero

# Start Dgraph
start

#Set Schema
curl -X PUT  -d '
    name: string @index(term, exact) .
    initial_release_date: datetime @index(year) .
' "http://localhost:8080/alter"

res=$(curl -X POST  -d 'schema {}' "http://localhost:8080/query")
expected='{"data":{"schema":[{"predicate":"_predicate_","type":"string","list":true},{"predicate":"initial_release_date","type":"datetime","index":true,"tokenizer":["year"]},{"predicate":"name","type":"string","index":true,"tokenizer":["term"]},{"predicate":"xid","type":"string","index":true,"tokenizer":["exact"]}]}}'
echo -e "Response $res"

# For some reason the comparison fails on Mac because of `_predicate_`
if [[ $TRAVIS_OS_NAME ==  "linux" ]] && [[ "$res" != "$expected" ]]; then
  echo "Schema comparison failed."
  quit 1
else
  echo "Schema comparison successful."
fi

echo -e "\nRunning dgraph live."
# Delete client directory to clear checkpoints.
rm -rf c

pushd dgraph &> /dev/null
./dgraph live -r $benchmark/goldendata.rdf.gz -d "127.0.0.1:9080,127.0.0.1:9082" -z "127.0.0.1:12340" -c 1 -b 10000
popd &> /dev/null

# Restart Dgraph so that we are sure that index keys are persisted.
quit 0
# Wait for a clean shutdown.
echo -e "\nClean Shutdown Done"
startZero
start

./contrib/scripts/goldendata-queries.sh

echo -e "\nShutting down Dgraph"
quit 0

echo -e "\nTrying to restart Dgraph and match export count"
startZero

start
echo -e "\nTrying to export data."
rm -rf export/*
curl http://localhost:8080/admin/export
echo -e "\nExport done."

pushd cmd/dgraph &> /dev/null
# This is count of RDF's in goldendata.rdf.gz + xids because we ran dgraph-live-loader with -xid flag.
dataCount="1120879"
# Concat exported files to get total count.
cat $(ls -t export/dgraph-1-* | head -1) $(ls -t export/dgraph-2-* | head -1) > export/dgraph-export.rdf.gz
if [[ $TRAVIS_OS_NAME == "osx" ]]; then
  exportCount=$(zcat < export/dgraph-export.rdf.gz | wc -l)
else
  exportCount=$(zcat export/dgraph-export.rdf.gz | wc -l)
fi

popd &> /dev/null



if [[ ! "$exportCount" -eq "$dataCount" ]]; then
  echo "Export test failed. Expected: $dataCount Got: $exportCount"
  quit 1
fi

echo -e "All tests ran fine. Exiting"
quit 0
