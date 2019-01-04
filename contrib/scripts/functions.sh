#!/bin/bash

function restartCluster {
  dir=$1
  zeroPort=$2
  alphaPort=$3
  dataDir=$4
  pushd $dir
    sudo rm -Rf $dataDir
    sudo mkdir $dataDir
    go build . && go install . && md5sum dgraph $GOPATH/bin/dgraph
    docker-compose down
    DATA="/tmp/dg" docker-compose up --force-recreate --remove-orphans --detach
  popd
  $GOPATH/src/github.com/dgraph-io/dgraph/contrib/wait-for-it.sh -t 60 localhost:$zeroPort
  $GOPATH/src/github.com/dgraph-io/dgraph/contrib/wait-for-it.sh -t 60 localhost:$alphaPort
  sleep 10 # Sleep 10 seconds to get things ready.
}

function stopCluster {
  dir=$1
  pushd $dir
    docker-compose down
  popd
}
