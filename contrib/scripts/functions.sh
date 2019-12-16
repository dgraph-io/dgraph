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
  if [[ "$OSTYPE" == "darwin"* ]]; then
    (env GOOS=linux GOARCH=amd64 go build) && mv -f dgraph $(go env GOPATH)/bin/dgraph
  else
    make install
  fi
  docker ps -a --filter label="cluster=test" --format "{{.Names}}" | xargs -r docker rm -f
  docker-compose -p dgraph -f $compose_file up --force-recreate --remove-orphans --detach || exit 1
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