#!/bin/bash

trap "cleanup" EXIT

cleanup() {
  killall -9 dgraph >/dev/null 2>/dev/null
}

ALPHA=./alpha_tls.sh
LIVE=./live_tls.sh
EXPECTED=1

$DGRAPH_BIN zero -w zw -o 1 > zero.log 2>&1 &
sleep 5

# start the server
$ALPHA > /dev/null 2>&1 &
timeout 30s $LIVE > /dev/null 2>&1
RESULT=$?

# regenerate TLS certificate
rm -f ./tls/ca.key
$DGRAPH_BIN cert -d $PWD/tls -n localhost -c live --force
pkill -HUP dgraph > /dev/null 2>&1

# try to connect again
timeout 30s $LIVE > /dev/null 2>&1
RESULT=$?

if [ $RESULT == $EXPECTED ]; then
	exit 0
else
	echo "Error while reloading TLS certificate"
	exit 1
fi
