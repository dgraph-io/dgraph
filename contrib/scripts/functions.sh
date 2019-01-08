#!/bin/bash

function restartCluster {
  basedir=$GOPATH/src/github.com/dgraph-io/dgraph
  pushd $basedir/dgraph
    go build . && go install . && md5sum dgraph $GOPATH/bin/dgraph
    docker-compose down
    DATA="/tmp/dg" docker-compose up --force-recreate --remove-orphans --detach
  popd
  $basedir/contrib/wait-for-it.sh -t 60 localhost:6080
  $basedir/contrib/wait-for-it.sh -t 60 localhost:9180
  sleep 10 # Sleep 10 seconds to get things ready.
}

function stopCluster {
  basedir=$GOPATH/src/github.com/dgraph-io/dgraph
  pushd $basedir/dgraph
    docker-compose down
  popd
}
