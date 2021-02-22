#!/bin/bash

mkdir -p yxjcluster_zero
mkdir -p yxjcluster_zero/log_zero1

./dgraph/dgraph zero --replicas=3 --raft="idx=1" --my=127.0.0.1:5080 --port_offset=0 --wal=./yxjcluster_zero/zw1 --v=2 --bindall --log_dir=./yxjcluster_zero/log_zero1 --force_new_cluster=true &
