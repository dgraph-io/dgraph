#!/bin/bash

export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y build-essential git golang
cd /dgraph/dgraph || exit
make
