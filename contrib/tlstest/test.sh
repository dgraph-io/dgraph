#!/bin/bash

SERVER=$1
CLIENT=$2
EXPECTED=$3

$SERVER > /dev/null 2>&1 &
P=$!
sleep 10
$CLIENT > /dev/null 2>&1
RESULT=$?
pkill dgraph > /dev/null 2>&1
rm -rf p w

echo "$SERVER <-> $CLIENT: $RESULT (expected: $EXPECTED)"
