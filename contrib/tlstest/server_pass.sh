#!/bin/bash

../../dgraph/dgraph alpha --tls "cacert=ca.crt; cert=server_pass.crt; key=server_pass.key;" --zero 127.0.0.1:5081 &> dgraph.log
