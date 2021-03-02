#!/bin/bash

../../dgraph/dgraph alpha --tls "ca-cert=ca.crt; client-cert=server.crt; client-key=server.key; client-auth-type=REQUIREANDVERIFY;" --zero 127.0.0.1:5081
