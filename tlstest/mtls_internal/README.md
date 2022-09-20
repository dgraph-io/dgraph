# Updating TLS certificates

Follow these instructions to update expired client and node certificates. Additional instructions may be added for ca certificates.

### Steps:

1. Copy ca.crt and ca.key to the location where you want to create the crts. They will be used to self sign client and node certificates.

2. Create two .cnf files that contain extensions for client.crt and node.crt each. They are as follows:

client.cnf:
[req]
req_extensions = v3_req
[ v3_req ]
basicConstraints = CA:FALSE
keyUsage = critical, digitalSignature
extendedKeyUsage = clientAuth

node.cnf:
[req]
req_extensions = v3_req
distinguished_name = req_distinguished_name
[req_distinguished_name]
[ v3_req ]
basicConstraints = CA:FALSE
keyUsage = digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth, clientAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = alpha1
DNS.2 = alpha2
DNS.3 = alpha3
DNS.4 = alpha4
DNS.5 = alpha5
DNS.6 = alpha6
DNS.7 = zero1
DNS.8 = zero2
DNS.9 = zero3
DNS.10 = localhost

3. execute these commands:
a. for alpha1/client (this creates client key/crt file)
openssl genrsa -out client.alpha1.key 2048

openssl req -new -key client.alpha1.key -out client.alpha1.csr -subj '/CN=Dgraph Root CA' -config client.cnf

openssl x509 -req -in client.alpha1.csr -CA ca.crt     -CAkey ca.key -CAcreateserial     -out client.alpha1.crt -days 365 -extensions v3_req     -extfile client.cnf

b. for alpha2/client (this creates client key/crt file)
openssl genrsa -out client.alpha2.key 2048

openssl req -new -key client.alpha2.key -out client.alpha2.csr -subj '/CN=Dgraph Root CA' -config client.cnf

openssl x509 -req -in client.alpha2.csr -CA ca.crt     -CAkey ca.key -CAcreateserial     -out client.alpha2.crt -days 365 -extensions v3_req     -extfile client.cnf

c. for alpha3/client (this creates client key/crt file)
openssl genrsa -out client.alpha3.key 2048

openssl req -new -key client.alpha3.key -out client.alpha3.csr -subj '/CN=Dgraph Root CA' -config client.cnf

openssl x509 -req -in client.alpha3.csr -CA ca.crt     -CAkey ca.key -CAcreateserial     -out client.alpha3.crt -days 365 -extensions v3_req     -extfile client.cnf

d. for alpha4/client (this creates client key/crt file)
openssl genrsa -out client.alpha4.key 2048

openssl req -new -key client.alpha4.key -out client.alpha4.csr -subj '/CN=Dgraph Root CA' -config client.cnf

openssl x509 -req -in client.alpha4.csr -CA ca.crt     -CAkey ca.key -CAcreateserial     -out client.alpha4.crt -days 365 -extensions v3_req     -extfile client.cnf

e. for alpha5/client (this creates client key/crt file)
openssl genrsa -out client.alpha5.key 2048

openssl req -new -key client.alpha5.key -out client.alpha5.csr -subj '/CN=Dgraph Root CA' -config client.cnf

openssl x509 -req -in client.alpha5.csr -CA ca.crt     -CAkey ca.key -CAcreateserial     -out client.alpha5.crt -days 365 -extensions v3_req     -extfile client.cnf

f. for alpha6/client (this creates client key/crt file)
openssl genrsa -out client.alpha6.key 2048

openssl req -new -key client.alpha6.key -out client.alpha6.csr -subj '/CN=Dgraph Root CA' -config client.cnf

openssl x509 -req -in client.alpha6.csr -CA ca.crt     -CAkey ca.key -CAcreateserial     -out client.alpha6.crt -days 365 -extensions v3_req     -extfile client.cnf

g. for live/client (this creates client key/crt file)
openssl genrsa -out client.liveclient.key 2048

openssl req -new -key client.liveclient.key -out client.liveclient.csr -subj '/CN=Dgraph Root CA' -config client.cnf

openssl x509 -req -in client.liveclient.csr -CA ca.crt     -CAkey ca.key -CAcreateserial     -out client.liveclient.crt -days 365 -extensions v3_req     -extfile client.cnf

h. for zero1/client (this creates client key/crt file)
openssl genrsa -out client.zero1.key 2048

openssl req -new -key client.zero1.key -out client.zero1.csr -subj '/CN=Dgraph Root CA' -config client.cnf

openssl x509 -req -in client.zero1.csr -CA ca.crt     -CAkey ca.key -CAcreateserial     -out client.zero1.crt -days 365 -extensions v3_req     -extfile client.cnf

i. for zero2/client (this creates client key/crt file)
openssl genrsa -out client.zero2.key 2048

openssl req -new -key client.zero2.key -out client.zero2.csr -subj '/CN=Dgraph Root CA' -config client.cnf

openssl x509 -req -in client.zero2.csr -CA ca.crt     -CAkey ca.key -CAcreateserial     -out client.zero2.crt -days 365 -extensions v3_req     -extfile client.cnf

j. for zero3/client (this creates client key/crt file)
openssl genrsa -out client.zero3.key 2048

openssl req -new -key client.zero3.key -out client.zero3.csr -subj '/CN=Dgraph Root CA' -config client.cnf

openssl x509 -req -in client.zero3.csr -CA ca.crt     -CAkey ca.key -CAcreateserial     -out client.zero3.crt -days 365 -extensions v3_req     -extfile client.cnf

4. Copy ONLY the .crt and .key files to the project.

5. For the node crts and keys, create one of each at a time and copy them(ONLY the .crt and .key files) to the project. Repeat this process 9 times to replace all the expired .crts and .keys. All the node crts and keys are distinct. Do not copy one instance of node.key and node.crt in all the required subfolders.

sudo openssl genrsa -out node.key 2048

sudo openssl req -new -key node.key     -out node.csr     -subj '/CN=Dgraph Node' -config node.cnf

sudo openssl x509 -req -in node.csr -CA ca.crt     -CAkey ca.key -CAcreateserial     -out node.crt -days 365 -extensions v3_req     -extfile node.cnf 
 


