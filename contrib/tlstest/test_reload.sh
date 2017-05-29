#!/bin/bash

SERVER=./server_reload.sh
CLIENT=./client_nopass.sh
EXPECTED=1

cp server.crt server_reload.crt
cp server.key server_reload.key

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

rm -rf p w


if [ $RESULT == $EXPECTED ]; then
	echo "TLS certificate reloaded successfully"
	exit 0
else
	echo "Error while reloading TLS certificate"
	exit 1
fi
