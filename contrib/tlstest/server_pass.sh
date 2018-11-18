#!/bin/bash

../../dgraph/dgraph alpha --tls_on --tls_ca_certs ca.crt --tls_cert server_pass.crt --tls_cert_key server_pass.key --tls_cert_key_passphrase secret --lru_mb 2048 --zero 127.0.0.1:5081 &> dgraph.log
