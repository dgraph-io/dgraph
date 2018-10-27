#!/bin/bash
trap "cleanup" EXIT

cleanup() {
  killall -KILL dgraph >/dev/null 2>/dev/null
}

ALPHA=$1
LIVE=$2
EXPECTED=$3

$DGRAPH_BIN zero -w zw -o 1 > zero.log 2>&1 &
sleep 5

$ALPHA >/dev/null 2>&1 &

if [ "x$RELOAD_TEST" != "x" ]; then
  trap '' HUP
  rm -f ./tls/ca.key
  $DGRAPH_BIN cert -d $PWD/tls -n localhost -c live --force
  killall -HUP dgraph >/dev/null 2>/dev/null
  sleep 3
fi

timeout 30s $LIVE > live.log 2>&1
RESULT=$?

if [ $RESULT != $EXPECTED ]; then
  echo "$ALPHA <-> $LIVE, Result: $RESULT != Expected: $EXPECTED"
  exit 1
fi

exit 0
