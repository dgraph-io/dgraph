#!/bin/bash

../../dgraph/dgraph server --tls.on --tls.ca_certs ca.crt --tls.cert server.crt --tls.cert_key server.key --tls.client_auth REQUIREANDVERIFY --memory_mb 2048 --zero 127.0.0.1:7081
