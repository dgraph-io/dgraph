#!/bin/bash
# Containers MUST be labeled with "cluster:test" to be restarted and stopped
# by these functions.

set -e

# May be called with an argument which is a docker compose file
# to use *instead of* the default docker-compose.yml.
function restartCluster {
  if [[ -z $1 ]]; then
    compose_file="docker-compose.yml"
  else
    compose_file="$(readlink -f $1)"
  fi

  basedir=$(dirname "${BASH_SOURCE[0]}")/../..
  pushd $basedir/dgraph >/dev/null
  echo "Rebuilding dgraph ..."

  docker_compose_gopath="${GOPATH:-$(go env GOPATH)}"
  make install

  if [[ "$OSTYPE" == "darwin"* ]]; then
    if !(AVAILABLE_RAM=$(cat ~/Library/Group\ Containers/group.com.docker/settings.json | grep memoryMiB | grep -oe "[0-9]\+") && test $AVAILABLE_RAM -ge 6144); then
      echo -e "\e[33mWarning: You may not have allocated enough memory for Docker on Mac. Please increase the allocated RAM to at least 6GB with a 4GB swap. See https://docs.docker.com/docker-for-mac/#resources \e[0m"
    fi
    docker_compose_gopath=`pwd`/../osx-docker-gopath

    # FIXME: read the go version from a constant
    docker run --rm \
      -v dgraph_gopath:/go \
      -v dgraph_gocache:/root/.cache/go-build \
      -v `pwd`/..:/app \
      -w /app/dgraph \
      golang:1.14 \
      go build -o /app/osx-docker-gopath/bin/dgraph
  fi

  docker ps -a --filter label="cluster=test" --format "{{.Names}}" | xargs -r docker rm -f
  GOPATH=$docker_compose_gopath docker-compose -p dgraph -f $compose_file up --force-recreate --remove-orphans -d || exit 1
  popd >/dev/null

  $basedir/contrib/wait-for-it.sh -t 60 localhost:6180 || exit 1
  $basedir/contrib/wait-for-it.sh -t 60 localhost:9180 || exit 1
  sleep 10 || exit 1
}

function stopCluster {
  docker ps --filter label="cluster=test" --format "{{.Names}}" \
  | xargs -r docker stop | sed 's/^/Stopped /'
  docker ps -a --filter label="cluster=test" --format "{{.Names}}" \
  | xargs -r docker rm | sed 's/^/Removed /'
}

function loginWithGroot() {
  curl -s -XPOST localhost:8180/login -d '{"userid": "groot","password": "password"}' \
   | python -c \
   "import json; resp = raw_input(); data = json.loads(resp); print data['data']['accessJWT']"
}
