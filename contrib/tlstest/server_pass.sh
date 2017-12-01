#!/bin/bash

../../dgraph/dgraph server --tls_on --tls_ca_certs ca.crt --tls_cert server_pass.crt --tls_cert_key server_pass.key --tls_cert_key_passphrase secret --memory_mb 2048 --zero 127.0.0.1:7081 &> dgraph.log
