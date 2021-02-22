#!/bin/bash

mkdir -p yxjcluster_alpha
mkdir -p yxjcluster_alpha/log_alpha1
mkdir -p yxjcluster_alpha/log_alpha4
mkdir -p yxjcluster_alpha/log_alpha5

./dgraph/dgraph alpha --raft="group=1" --my=127.0.0.1:7080 --port_offset=0 --zero=127.0.0.1:5080 --postings=./yxjcluster_alpha/p1 --wal=./yxjcluster_alpha/w1 --v=2 --bindall --log_dir=./yxjcluster_alpha/log_alpha1 &

./dgraph/dgraph alpha --raft="group=1" --my=127.0.0.1:7083 --port_offset=3 --zero=127.0.0.1:5080 --postings=./yxjcluster_alpha/p4 --wal=./yxjcluster_alpha/w4 --v=2 --bindall --log_dir=./yxjcluster_alpha/log_alpha4 &

./dgraph/dgraph alpha --raft="group=1" --my=127.0.0.1:7084 --port_offset=4 --zero=127.0.0.1:5080 --postings=./yxjcluster_alpha/p5 --wal=./yxjcluster_alpha/w5 --v=2 --bindall --log_dir=./yxjcluster_alpha/log_alpha5 &
