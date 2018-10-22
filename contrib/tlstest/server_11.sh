#!/bin/bash

../../dgraph/dgraph alpha --tls_on --tls_ca_certs ca.crt --tls_cert server.crt --tls_cert_key server.key --tls_max_version=TLS11 --lru_mb 2048 --zero 127.0.0.1:5080
