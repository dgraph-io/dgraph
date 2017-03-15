#!/bin/bash

../../cmd/dgraphloader/dgraphloader -d server1.dgraph.io:8080 -tls.on -tls.ca_certs ca.crt -tls.cert client.crt -tls.cert_key client.key -tls.server_name server1.dgraph.io -tls.min_version=TLS12 -r data.rdf.gz 
