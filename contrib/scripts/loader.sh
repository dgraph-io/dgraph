#!/bin/bash

basedir=$GOPATH/src/github.com/dgraph-io/dgraph
contrib=$basedir/contrib
builddir=$basedir/dgraph

set -e

pushd $builddir
go build . && go install . && md5sum dgraph $GOPATH/bin/dgraph
# docker-compose up --force-recreate --remove-orphans
popd

# function finish {
#   echo "Killing $pid"
#   kill $pid
#   sleep 10
#   if ps -p $pid > /dev/null
#   then
#     echo "Compose still running. Killing it."
#     kill -9 $pid
#   else
#     echo "Compose down. Exiting."
#   fi
# }

# trap 'finish' EXIT

# source $contrib/scripts/functions.sh

# SRC="$( cd -P "$( dirname "${BASH_SOURCE[0]}" )" && pwd )/.."
# echo $SRC

# BUILD=$1
# # If build variable is empty then we set it.
# if [ -z "$1" ]; then
#   BUILD=$SRC/build
# fi

# echo "BUILD: $BUILD"
# mkdir -p $BUILD
# pushd $BUILD &> /dev/null

mkdir -p tmp
pushd tmp
echo "Inside"
pwd
rm -f *

if [ ! -f "goldendata.rdf.gz" ]; then
  cp $basedir/systest/data/goldendata.rdf.gz .
fi

# log file size.
ls -la goldendata.rdf.gz

echo "HERE ***********************************"

# benchmark=$(pwd)
# popd &> /dev/null

# startZero
# Start Dgraph
# start

#Set Schema
curl -XPOST  -d '
    name: string @index(term) @lang .
    initial_release_date: datetime @index(year) .
' "http://localhost:8180/alter"

echo -e "\nRunning dgraph live."
# Delete client directory to clear xidmap.

# rm -rf $BUILD/xiddir
dgraph live -r goldendata.rdf.gz -d "127.0.0.1:9180" -z "127.0.0.1:5080" -c 1 -b 1000
popd
rm -Rf tmp

# Restart Dgraph so that we are sure that index keys are persisted.
# quit 0

# startZero
# start

# $contrib/scripts/goldendata-queries.sh

# quit 0
