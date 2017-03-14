#!/bin/bash

../../cmd/dgraph/dgraph -tls.on -tls.ca_certs ca.crt -tls.cert server.crt -tls.cert_key server.key -tls.max_version=TLS11
