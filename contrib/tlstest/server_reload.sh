#!/bin/bash

../../cmd/dgraph/dgraph -tls.on -tls.ca_certs ca.crt -tls.cert server_reload.crt -tls.cert_key server_reload.key \
	--memory_mb 2048 --peer 127.0.0.1:8888
