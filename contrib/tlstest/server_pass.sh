#!/bin/bash

../../dgraph/dgraph alpha --tls "ca-cert=ca.crt; client-cert=server_pass.crt; client-key=server_pass.key;" --zero 127.0.0.1:5081 &> dgraph.log
