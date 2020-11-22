#!/bin/bash
set -e
$DGRAPH_BIN live -d localhost:9080 --tls_dir $PWD/tls --tls_server_name localhost -r data.rdf.gz -z 127.0.0.1:5081
