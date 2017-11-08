#!/bin/bash

function quit {
  curl localhost:8080/admin/shutdown > /dev/null
  curl localhost:8082/admin/shutdown > /dev/null
  # Kill Dgraphzero
  killall  dgraph

  return $1
}

function start {
  pushd dgraph &> /dev/null
  echo -e "Starting first server.\n"
  ./dgraph server -p $BUILD/p -w $BUILD/w --memory_mb 2048 --idx 1 --my "127.0.0.1:12345" --zero "127.0.0.1:12340"> $BUILD/server.log &
  sleep 5
  echo -e "Starting second server.\n"
  ./dgraph server -p $BUILD/p2 -w $BUILD/w2 --memory_mb 2048 --idx 2 --my "127.0.0.1:12346" --zero "127.0.0.1:12340" --port 8082 --grpc_port 9082 --workerport 12346 > $BUILD/server2.log &
  # Wait for membership sync to happen.
	# TODO: Change this to wait for health check.
  sleep 5
  popd &> /dev/null
  return 0
}

function startZero {
  pushd dgraph &> /dev/null
  echo -e "\nBuilding Dgraph Zero."
  go build .
	echo -e "Starting dgraph zero.\n"
  ./dgraph zero --w $BUILD/wz --port 12340 &
  # To ensure dgraph doesn't start before dgraphzero.
	sleep 5
  popd &> /dev/null
}
