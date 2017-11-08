#!/bin/bash

function quit {
	killall -9 dgraphzero
  curl localhost:8080/admin/shutdown
  curl localhost:8082/admin/shutdown

  while pgrep dgraph;
  do
    sleep 5
  done
  return $1
}

function start {
  pushd cmd/dgraph &> /dev/null
  echo -e "\nBuilding Dgraph."
  go build .

  echo -e "Starting first server.\n"
  ./dgraph -p $BUILD/p -w $BUILD/w -memory_mb 2048 --idx 1 --my "127.0.0.1:12345" --zero "127.0.0.1:12340"> $BUILD/server.log &
  sleep 5
  echo -e "Starting second server.\n"
  ./dgraph -p $BUILD/p2 -w $BUILD/w2 -memory_mb 2048 --idx 2 --my "127.0.0.1:12346" --zero "127.0.0.1:12340" --port 8082 --grpc_port 9082 --workerport 12346 > $BUILD/server2.log &
  # Wait for membership sync to happen.
	# TODO: Change this to wait for health check.
  sleep 10
  popd &> /dev/null
  return 0
}

function startZero {
  pushd cmd/dgraphzero &> /dev/null
  echo -e "\nBuilding Dgraph Zero."
  go build .
	echo -e "Staring dgraph zero.\n"
  ./dgraphzero -w $BUILD/wz -port 12340 &
  # To ensure dgraph doesn't start before dgraphzero.
	sleep 5
  popd &> /dev/null
}
