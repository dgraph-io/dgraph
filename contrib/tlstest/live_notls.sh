#!/bin/bash
set -e
$DGRAPH_BIN live -d localhost:9080 -r data.rdf.gz -z 127.0.0.1:5081
