#!/bin/bash

files=goldendata.rdf.gz
dgraph="127.0.0.1:8080"
# dgraph="play.dgraph.io:80"
concurrent=1
numRdf=1

tlsEnabled="false"
tlsInsecure="false"
tlsMinVersion="TLS12"
tlsMaxVersion="TLS12"
tlsServerName="git.tarrovs.xyz"
tlsCert="../test_data/cert_pass_signed.crt"
tlsKey="../test_data/cert_pass.key"
# tlsCert=""
# tlsKey=""
# tlsCert="../test_data/tarrovs.xyz.crt"
# tlsKey="../test_data/tarrovs.xyz.key"
# tlsCert="client.crt"
# tlsKey="client.key"
tlsKeyPass="test"
tlsRootCACerts="../test_data/letsencrypt.pem"
# tlsRootCACerts="../test_data/cert.crt"
# tlsRootCACerts=""
tlsSystemCACerts="false"

echo "./dgraphloader -r $files -d $dgraph -c $concurrent -m $numRdf -tls.on=$tlsEnabled -tls.insecure=$tlsInsecure -tls.server_name $tlsServerName -tls.min_version $tlsMinVersion -tls.max_version $tlsMaxVersion \
-tls.cert $tlsCert -tls.cert_key $tlsKey -tls.cert_key_passphrase $tlsKeyPass -tls.ca_certs $tlsRootCACerts -tls.use_system_ca=$tlsSystemCACerts"

./dgraphloader -r $files -d $dgraph -c $concurrent -m $numRdf -tls.on=$tlsEnabled -tls.insecure=$tlsInsecure \
-tls.server_name "$tlsServerName" -tls.min_version "$tlsMinVersion" -tls.max_version "$tlsMaxVersion" \
-tls.cert "$tlsCert" -tls.cert_key "$tlsKey" -tls.cert_key_passphrase "$tlsKeyPass" -tls.ca_certs "$tlsRootCACerts" \
-tls.use_system_ca=$tlsSystemCACerts
