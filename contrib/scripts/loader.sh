#!/bin/bash

basedir=$GOPATH/src/github.com/dgraph-io/dgraph
set -e

source $basedir/contrib/scripts/functions.sh
startCluster

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

echo "Setting schema."
while true; do
  curl -s -XPOST --output alter.txt -d '
      name: string @index(term) @lang .
      initial_release_date: datetime @index(year) .
  ' "http://localhost:8180/alter"
  cat alter.txt
  echo
  cat alter.txt | grep -iq "success" && break
  echo "Retrying..."
  sleep 3
done
rm -f alter.txt

echo -e "\nRunning dgraph live."
dgraph live -r goldendata.rdf.gz -d "127.0.0.1:9180" -z "127.0.0.1:5080" -c 10
popd
rm -Rf tmp

echo "Running queries"
$basedir/contrib/scripts/goldendata-queries.sh
