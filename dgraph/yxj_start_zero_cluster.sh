#!/bin/bash

mkdir -p yxjcluster_zero
mkdir -p yxjcluster_zero/log_zero1
mkdir -p yxjcluster_zero/log_zero2
mkdir -p yxjcluster_zero/log_zero3

./dgraph/dgraph zero --replicas=3 --raft="idx=1" --my=127.0.0.1:5080 --port_offset=0 --wal=./yxjcluster_zero/zw1 --v=2 --bindall --log_dir=./yxjcluster_zero/log_zero1 &

./dgraph/dgraph zero --replicas=3 --raft="idx=2" --my=127.0.0.1:5081 --port_offset=1 --peer=127.0.0.1:5080 --wal=./yxjcluster_zero/zw2 --v=2 --bindall --log_dir=./yxjcluster_zero/log_zero2 &

./dgraph/dgraph zero --replicas=3 --raft="idx=3" --my=127.0.0.1:5082 --port_offset=2 --peer=127.0.0.1:5080 --wal=./yxjcluster_zero/zw3 --v=2 --bindall --log_dir=./yxjcluster_zero/log_zero3 &
