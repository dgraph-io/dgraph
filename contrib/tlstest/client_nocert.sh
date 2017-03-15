#!/bin/bash

../../cmd/dgraphloader/dgraphloader -d server1.dgraph.io:8080 -tls.on -tls.ca_certs ca.crt -tls.server_name server1.dgraph.io -r data.rdf.gz 
