#!/bin/bash
set -e
$DGRAPH_BIN alpha --tls_dir $PWD/tls --tls_client_auth REQUIREANDVERIFY --zero 127.0.0.1:5081 &> alpha.log
