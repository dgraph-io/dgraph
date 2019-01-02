#!/bin/bash

# may be called with an argument which is a docker compose file
# to use *in addition* to the default docker-compose.yml
function restartCluster {
  local compose_files=("-f" "docker-compose.yml")
  if [[ -n $1 ]]; then
    compose_files+=("-f" "$(readlink -f $1)")
  fi

  basedir=$GOPATH/src/github.com/dgraph-io/dgraph
  pushd $basedir/dgraph
    go build . && go install . && md5sum dgraph $GOPATH/bin/dgraph
    docker-compose --log-level=CRITICAL down
    docker-compose ${compose_files[@]} up --force-recreate --remove-orphans --detach
  popd

  $basedir/contrib/wait-for-it.sh -t 60 localhost:6080 || exit 1
  $basedir/contrib/wait-for-it.sh -t 60 localhost:9180 || exit 1
  sleep 10 || exit 1	# Sleep 10 seconds to get things ready.
}

function stopCluster {
  basedir=$GOPATH/src/github.com/dgraph-io/dgraph
  pushd $basedir/dgraph
    docker-compose -f docker-compose.yml down
  popd
}
