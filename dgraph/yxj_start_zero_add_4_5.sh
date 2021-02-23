#!/bin/bash

mkdir -p yxjcluster_zero
mkdir -p yxjcluster_zero/log_zero4
mkdir -p yxjcluster_zero/log_zero5

./dgraph/dgraph zero --replicas=3 --raft="idx=4" --my=127.0.0.1:5083 --port_offset=3 --peer=127.0.0.1:5080 --wal=./yxjcluster_zero/zw4 --v=2 --bindall --log_dir=./yxjcluster_zero/log_zero4 &

./dgraph/dgraph zero --replicas=3 --raft="idx=5" --my=127.0.0.1:5084 --port_offset=4 --peer=127.0.0.1:5080 --wal=./yxjcluster_zero/zw5 --v=2 --bindall --log_dir=./yxjcluster_zero/log_zero5 &
