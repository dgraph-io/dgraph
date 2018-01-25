#!/bin/bash

../../dgraph/dgraph server --tls_on --tls_ca_certs ca.crt --tls_cert server.crt --tls_cert_key server.key --tls_max_version=TLS11 --memory_mb 2048 --zero 127.0.0.1:5080
