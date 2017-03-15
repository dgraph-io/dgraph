KEYBITS=2048
PASS=secret

KEYS=ca.key server.key client.key server_pass.key client_pass.key
CERTS=ca.crt server.crt client.crt server_pass.crt client_pass.crt

test: all
	@echo -e "\n\n\nTESTING...\n"
	@echo "Basic TLS communication, unencrypted keys"
	@./test.sh ./server_nopass.sh ./client_nocert.sh 0
	@echo "Basic TLS communication, encrypted keys (with password)"
	@./test.sh ./server_pass.sh ./client_pass.sh 0
	@echo "TLS client authentication, unencrypted key"
	@./test.sh ./server_nopass_client_auth.sh ./client_nopass.sh 0
	@echo "TLS client authentication, no client cert (failure expected)"
	@./test.sh ./server_nopass_client_auth.sh ./client_nocert.sh 1
	@echo "Invalid server name (failure expected)"
	@./test.sh ./server_nopass.sh ./client_pass.sh 1
	@echo "TLS protocol versions mismatch (failure expected)"
	@./test.sh ./server_11.sh ./client_12.sh 1

all: ca $(KEYS) $(CERTS)

clean:
	rm -rf newcerts index.* serial *.old *.key *.csr *.crt p w

ca:
	@mkdir -p newcerts
	@touch index.txt
	@touch index.txt.attr
	@echo 1000 > serial

ca.crt: ca.key
	openssl req -key ca.key -new -x509 -extensions v3_ca -out ca.crt -nodes -subj "/C=AU/L=Sydney/O=Dgraph/CN=ca.dgraph.io"

ca.key:
	openssl genrsa -out ca.key $(KEYBITS)

server.csr server.key:
	openssl req -new -newkey rsa:$(KEYBITS) -keyout server.key -out server.csr -nodes -subj "/C=AU/L=Sydney/O=Dgraph/CN=server1.dgraph.io"

client.csr client.key:
	openssl req -new -newkey rsa:$(KEYBITS) -keyout client.key -out client.csr -nodes -subj "/C=AU/L=Sydney/O=Dgraph/CN=client1.dgraph.io"

server_pass.key:
	openssl genrsa -aes256 -out server_pass.key -passout pass:$(PASS) $(KEYBITS)

client_pass.key:
	openssl genrsa -aes256 -out client_pass.key -passout pass:$(PASS) $(KEYBITS)

server_pass.csr: server_pass.key
	openssl req -new -key server_pass.key -out server_pass.csr -subj "/C=AU/L=Sydney/O=Dgraph/CN=server2.dgraph.io" -passin pass:$(PASS) -passout pass:$(PASS)

client_pass.csr: client_pass.key
	openssl req -new -key client_pass.key -out client_pass.csr -subj "/C=AU/L=Sydney/O=Dgraph/CN=client2.dgraph.io" -passin pass:$(PASS) -passout pass:$(PASS)

%.crt: %.csr ca.crt ca.key 
	openssl ca -config openssl.cnf -days 365 -notext -cert ca.crt -keyfile ca.key -in $< -out $@ -batch
