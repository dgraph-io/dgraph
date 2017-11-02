#!/bin/bash

../../cmd/dgraph/dgraph -tls.on -tls.ca_certs ca.crt -tls.cert server_pass.crt -tls.cert_key server_pass.key -tls.cert_key_passphrase secret --memory_mb 2048 --zero 127.0.0.1:8888
