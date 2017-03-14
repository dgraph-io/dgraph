#!/bin/bash

../../cmd/dgraphloader/dgraphloader -d server2.dgraph.io:8080 -tls.on -tls.ca_certs ca.crt -tls.cert client_pass.crt -tls.cert_key client_pass.key -tls.cert_key_passphrase secret -tls.server_name server2.dgraph.io -r data.rdf.gz 
