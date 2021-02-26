#!/bin/bash
set -e
$DGRAPH_BIN alpha --tls "cacert=$PWD/tls/ca.crt; node-cert=$PWD/tls/node.crt; node-key=$PWD/tls/node.key; client-auth=REQUIREANDVERIFY;" --zero 127.0.0.1:5081 &> alpha.log
