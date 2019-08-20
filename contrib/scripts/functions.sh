#!/bin/bash
# Containers MUST be labeled with "cluster:test" to be restarted and stopped
# by these functions.

# May be called with an argument which is a docker compose file
# to use *instead of* the default docker-compose.yml.
function restartCluster {
  if [[ -z $1 ]]; then
    compose_file="docker-compose.yml"
  else
    compose_file="$(readlink -f $1)"
  fi

  basedir=$GOPATH/src/github.com/dgraph-io/dgraph
  pushd $basedir/dgraph >/dev/null
  echo "Rebuilding dgraph ..."
  if [[ "$OSTYPE" == "darwin"* ]]; then
    (env GOOS=linux GOARCH=amd64 go build) && mv -f dgraph $GOPATH/bin/dgraph
  else
    make install
  fi
  docker ps -a --filter label="cluster=test" --format "{{.Names}}" | xargs -r docker rm -f
  docker-compose -p dgraph -f $compose_file up --force-recreate --remove-orphans --detach || exit 1
  popd >/dev/null

  $basedir/contrib/wait-for-it.sh -t 60 localhost:6180 || exit 1
  $basedir/contrib/wait-for-it.sh -t 60 localhost:9180 || exit 1

  # Wait for zero server to become the leader before applying enterprise license.
  applyLicense
	sleep 10 || exit 1
}

function applyLicense {
  n=0
  until [ $n -ge 5 ]
  do
    echo "Applying license to Zero"
 	  status_code=$(curl -X POST --write-out "%{http_code}\n" --silent --output /dev/null http://localhost:6180/enterpriseLicense \
  		--header 'Content-Type: application/json' \
  		--data-binary "$LICENSE_FILE")

    if [[ "$status_code" -ne 200 ]] ; then
    	echo "Got $status_code while applying license. Sleeping for 5 secs."
  	  sleep 5
    else
  		break
		fi
    n=$[$n+1]
  done
}

function stopCluster {
  basedir=$GOPATH/src/github.com/dgraph-io/dgraph
  pushd $basedir/dgraph >/dev/null
  docker ps --filter label="cluster=test" --format "{{.Names}}" \
  | xargs -r docker stop | sed 's/^/Stopped /'
  docker ps -a --filter label="cluster=test" --format "{{.Names}}" \
  | xargs -r docker rm | sed 's/^/Removed /'
  popd >/dev/null
}
