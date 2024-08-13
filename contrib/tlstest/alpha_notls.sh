#!/bin/bash
set -e
$DGRAPH_BIN alpha --zero 127.0.0.1:5081 &> alpha.log
