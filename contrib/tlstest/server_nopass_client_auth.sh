#!/bin/bash

../../dgraph/dgraph alpha --tls "cacert=ca.crt; cert=server.crt; key=server.key; client-auth=REQUIREANDVERIFY;" --zero 127.0.0.1:5081
