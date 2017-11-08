#!/bin/bash
set -e

source ./contrib/scripts/functions.sh

BUILD=$1

startZero

# Start dgraph in the background.
start
sleep 5

# Wait for server to start in the background.
until nc -z 127.0.0.1 8080;
do
        sleep 1
done

go test -v ./contrib/freebase

quit 0

