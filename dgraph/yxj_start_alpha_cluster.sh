#!/bin/bash

mkdir -p yxjcluster_alpha
mkdir -p yxjcluster_alpha/log_alpha1
mkdir -p yxjcluster_alpha/log_alpha2
mkdir -p yxjcluster_alpha/log_alpha3

./dgraph/dgraph alpha --raft="group=1" --my=127.0.0.1:7080 --port_offset=0 --zero=127.0.0.1:5080 --postings=./yxjcluster_alpha/p1 --wal=./yxjcluster_alpha/w1 --v=2 --bindall --log_dir=./yxjcluster_alpha/log_alpha1 &

./dgraph/dgraph alpha --raft="group=1" --my=127.0.0.1:7081 --port_offset=1 --zero=127.0.0.1:5080 --postings=./yxjcluster_alpha/p2 --wal=./yxjcluster_alpha/w2 --v=2 --bindall --log_dir=./yxjcluster_alpha/log_alpha2 &

./dgraph/dgraph alpha --raft="group=1" --my=127.0.0.1:7082 --port_offset=2 --zero=127.0.0.1:5080 --postings=./yxjcluster_alpha/p3 --wal=./yxjcluster_alpha/w3 --v=2 --bindall --log_dir=./yxjcluster_alpha/log_alpha3 &
