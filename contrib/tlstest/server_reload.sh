#!/bin/bash

../../cmd/dgraph/dgraph -tls_on -tls_ca_certs ca.crt -tls_cert server_reload.crt -tls_cert_key server_reload.key \
	--memory_mb 2048 --zero 127.0.0.1:8888
