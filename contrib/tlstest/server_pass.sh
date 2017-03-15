#!/bin/bash

../../cmd/dgraph/dgraph -tls.on -tls.ca_certs ca.crt -tls.cert server_pass.crt -tls.cert_key server_pass.key -tls.cert_key_passphrase secret
