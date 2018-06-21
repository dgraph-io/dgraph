#!/bin/bash

basedir=$GOPATH/src/github.com/dgraph-io/dgraph
contrib=$basedir/contrib

set -e

# Build Dgraph and install it.
pushd $basedir/dgraph
go build . && go install . && md5sum dgraph $GOPATH/bin/dgraph
docker-compose up --force-recreate --remove-orphans --detach
popd

# Create a temporary directory to use for running live loader.
mkdir -p tmp
pushd tmp
echo "Inside `pwd`"
rm -f *

if [ ! -f "goldendata.rdf.gz" ]; then
  cp $basedir/systest/data/goldendata.rdf.gz .
fi

# log file size.
ls -la goldendata.rdf.gz

$contrib/wait-for-it.sh localhost:8180
sleep 5 # Give it 5 more seconds to be ready.

echo "Setting schema."
curl -XPOST  -d '
    name: string @index(term) @lang .
    initial_release_date: datetime @index(year) .
' "http://localhost:8180/alter"

echo -e "\nRunning dgraph live."
dgraph live -r goldendata.rdf.gz -d "127.0.0.1:9180" -z "127.0.0.1:5080" -c 1 -b 1000
popd
rm -Rf tmp

echo "Running queries"
$contrib/scripts/goldendata-queries.sh
