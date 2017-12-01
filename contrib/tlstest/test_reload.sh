#!/bin/bash

killall dgraph

SERVER=./server_reload.sh
CLIENT=./client_nopass.sh
EXPECTED=1

cp server.crt server_reload.crt
cp server.key server_reload.key


$GOPATH/src/github.com/dgraph-io/dgraph/dgraph/dgraph zero -w zw -o 1> /dev/null 2>&1 &
sleep 5

# start the server
$SERVER > /dev/null 2>&1 &
P=$!
timeout 30s $CLIENT > /dev/null 2>&1
RESULT=$?

# reload server certificate
cp server3.crt server_reload.crt
cp server3.key server_reload.key
pkill -HUP dgraph > /dev/null 2>&1

# try to connect again
timeout 30s $CLIENT > /dev/null 2>&1
RESULT=$?

if [ $RESULT == $EXPECTED ]; then
	echo "TLS certificate reloaded successfully"
	exit 0
else
	echo "Error while reloading TLS certificate"
	exit 1
fi
