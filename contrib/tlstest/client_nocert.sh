#!/bin/bash

../../dgraph/dgraph live -d server1.dgraph.io:9080 --tls.on --tls.ca_certs ca.crt --tls.server_name server1.dgraph.io -r data.rdf.gz -z 127.0.0.1:7081
