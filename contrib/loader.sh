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

sleep 10

#Set Schema
curl -X POST  -d 'mutation {
  schema {
    name: string @index(term) .
    xid: string @index(exact) .
    initial_release_date: datetime @index(year) .
  }
}' "http://localhost:8080/query"

echo -e "\nBuilding and running dgraphloader."
pushd cmd/dgraphloader &> /dev/null
# Delete client directory to clear checkpoints.
rm -rf c
go build .
./dgraphloader -r $benchmark/goldendata.rdf.gz -x true
popd &> /dev/null


# Restart Dgraph so that we are sure that index keys are persisted.
quit 0
# Wait for a clean shutdown.
pushd cmd/dgraph &> /dev/null
start
popd &> /dev/null

./contrib/goldendata-queries.sh

echo -e "\nShutting down Dgraph"
quit 0

echo -e "\nTrying to restart Dgraph and match export count"
pushd cmd/dgraph &> /dev/null
start
# Wait to become leader.
sleep 5
echo -e "\nTrying to export data."
curl http://localhost:8080/admin/export
echo -e "\nExport done."

# This is count of RDF's in goldendata.rdf.gz + xids because we ran dgraphloader with -xid flag.
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
