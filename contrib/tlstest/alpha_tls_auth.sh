#!/bin/bash
set -e
$DGRAPH_BIN alpha --tls "ca-cert=$PWD/tls/ca.crt; server-cert=$PWD/tls/node.crt; server-key=$PWD/tls/node.key; client-auth-type=REQUIREANDVERIFY;" --zero 127.0.0.1:5081 &> alpha.log
