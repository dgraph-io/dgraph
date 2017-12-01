#!/bin/bash

../../dgraph/dgraph live -d server2.dgraph.io:9080 --tls_on --tls_ca_certs ca.crt --tls_cert client_pass.crt --tls_cert_key client_pass.key --tls_cert_key_passphrase secret --tls_server_name server2.dgraph.io -r data.rdf.gz -z 127.0.0.1:7081
