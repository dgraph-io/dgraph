#!/bin/bash

basedir=$GOPATH/src/github.com/dgraph-io/dgraph
set -e

source $basedir/contrib/scripts/functions.sh
restartCluster

# Create a temporary directory to use for running live loader.
tmpdir=`mktemp --tmpdir -d loader.tmp-XXXXXX`
trap "rm -rf $tmpdir" EXIT
pushd $tmpdir
echo "Inside `pwd`"

ln -s $basedir/systest/data/goldendata.rdf.gz .

# log file size.
ls -laH goldendata.rdf.gz

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
dgraph live -f goldendata.rdf.gz -a "127.0.0.1:9180" -z "127.0.0.1:5080" -c 10
popd
rm -rf $tmpdir

echo "Running queries"
$basedir/contrib/scripts/goldendata-queries.sh
