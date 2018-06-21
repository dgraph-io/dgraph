#!/bin/bash

sleepTime=11

function runCluster {
  basedir=$GOPATH/src/github.com/dgraph-io/dgraph
  pushd $basedir/dgraph
    go build . && go install . && md5sum dgraph $GOPATH/bin/dgraph
    docker-compose up --force-recreate --remove-orphans --detach
  popd
  $basedir/contrib/wait-for-it.sh localhost:6080
  $basedir/contrib/wait-for-it.sh localhost:9180
}

# function quit {
#   echo "Shutting down dgraph server and zero"
#   curl -s localhost:8081/admin/shutdown
#   curl -s localhost:8082/admin/shutdown
#   # Kill Dgraphzero
#   kill -9 $(pgrep -f "dgraph zero") > /dev/null

#   if pgrep -x dgraph > /dev/null
#   then
#     while pgrep dgraph;
#     do
#       echo "Sleeping for 5 secs so that Dgraph can shutdown."
#       sleep 5
#     done
#   fi

#   echo "Clean shutdown done."
#   return $1
# }

# function start {
#   pushd dgraph &> /dev/null
#   echo -e "Starting first server."
#   ./dgraph server -p $BUILD/p -w $BUILD/w --lru_mb 4096 -o 1 &
#   sleep 5
#   echo -e "Starting second server.\n"
#   ./dgraph server -p $BUILD/p2 -w $BUILD/w2 --lru_mb 4096 -o 2 &
#   # Wait for membership sync to happen.
#   sleep $sleepTime
#   popd &> /dev/null
#   return 0
# }

# function startZero {
#   pushd dgraph &> /dev/null
#   echo -e "\nBuilding Dgraph."
#   go build .
# 	echo -e "Starting dgraph zero.\n"
#   ./dgraph zero -w $BUILD/wz &
#   # To ensure dgraph doesn't start before dgraphzero.
# 	# It takes time for zero to start on travis(mac).
# 	sleep $sleepTime
#   popd &> /dev/null
# }
