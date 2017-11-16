#!/bin/bash

../../dgraph/dgraph live -d server1.dgraph.io:9080 --tls.on --tls.ca_certs ca.crt --tls.cert client.crt --tls.cert_key client.key --tls.server_name server1.dgraph.io --tls.min_version=TLS12 -r data.rdf.gz -z 127.0.0.1:7081
