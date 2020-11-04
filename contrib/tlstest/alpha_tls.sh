#!/bin/bash
set -e
$DGRAPH_BIN alpha --tls_cacert $PWD/tls/ca.crt --tls_node_cert $PWD/tls/node.crt --tls_node_key $PWD/tls/node.key --zero 127.0.0.1:5081 &> alpha.log
