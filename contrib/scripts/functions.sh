#!/bin/bash

# may be called with an argument which is a docker compose file
# to use *in addition* to the default docker-compose.yml
function restartCluster {
  #compose_files=("-f" "docker-compose.yml")
  if [[ -n $1 ]]; then
    compose_files+=("-f" "$(readlink -f $1)")
  fi

  basedir=$GOPATH/src/github.com/dgraph-io/dgraph
  pushd $basedir/dgraph >/dev/null
    go build . && go install . && md5sum dgraph $GOPATH/bin/dgraph
    docker-compose --log-level=CRITICAL down
    docker-compose ${compose_files[@]} up \
                   --force-recreate --remove-orphans --detach
  popd >/dev/null

  $basedir/contrib/wait-for-it.sh -t 60 localhost:6080 || exit 1
  $basedir/contrib/wait-for-it.sh -t 60 localhost:9180 || exit 1
  echo "Waiting 10 seconds for cluster to come up"
  sleep 10 || exit 1
}

function stopCluster {
  if [[ -n $1 ]]; then
    compose_files+=("-f" "$(readlink -f $1)")
  fi

  basedir=$GOPATH/src/github.com/dgraph-io/dgraph
  pushd $basedir/dgraph >/dev/null
    docker-compose ${compose_files[@]} down
  popd >/dev/null
}
