#!/bin/bash

function quit {
  curl localhost:8080/admin/shutdown
  curl localhost:8082/admin/shutdown
  sleep 10
  return $1
}

function start {
  echo -e "Starting first server.\n"
  ./dgraph -p $BUILD/p -w $BUILD/w -memory_mb 2048 --group_conf groups.conf --groups "0,1" --idx 1 --my "127.0.0.1:12345" > $BUILD/server.log &
  sleep 5
  echo -e "Starting second server.\n"
  ./dgraph -p $BUILD/p2 -w $BUILD/w2 -memory_mb 2048 --group_conf groups.conf --groups "2" --idx 2 --my "127.0.0.1:12346" --peer "127.0.0.1:12345" --port 8082 --grpc_port 9082 --workerport 12346 > $BUILD/server2.log &
  return 0
}
