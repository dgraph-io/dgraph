# Updating TLS certificates

Follow these instructions to update expired client and node certificates. Additional instructions may be added for ca certificates.

### Steps:

1. Copy ca.crt and ca.key to the location where you want to create the crts.

2. Create two .cnf files that contain extensions for client.crt and node.crt each.

3. Execute these commands:
openssl genrsa -out client.alpha1.key 2048

openssl req -new -key client.alpha1.key -out client.alpha1.csr -subj '/CN=Dgraph Root CA' -config client.cnf

openssl x509 -req -in client.alpha1.csr -CA ca.crt     -CAkey ca.key -CAcreateserial     -out client.alpha1.crt -days 365 -extensions v3_req     -extfile client.cnf

repeat for all client and node certs.

