#!/bin/bash

function restartCluster {
  basedir=$GOPATH/src/github.com/dgraph-io/dgraph
  pushd $basedir/dgraph
    go build . && go install . && md5sum dgraph $GOPATH/bin/dgraph
    docker-compose --log-level=CRITICAL down
    docker-compose up --force-recreate --remove-orphans --detach
  popd

  $basedir/contrib/wait-for-it.sh -t 60 localhost:6080 || exit 1
  $basedir/contrib/wait-for-it.sh -t 60 localhost:9180 || exit 1
  echo "Waiting 10 seconds for cluster to come up"
  sleep 10 || exit 1
}

function stopCluster {
  basedir=$GOPATH/src/github.com/dgraph-io/dgraph
  pushd $basedir/dgraph
    docker-compose down
  popd
}
