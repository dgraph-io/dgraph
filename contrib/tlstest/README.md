# Semiautomatic tests of TLS configuration

This directory contains several scripts, that helps with testing of tls functionality in dgraph.

- `Makefile` - cleans up the directory, creates CA, client and server keys and signed certs, executes the tests
- `server_nopass.sh` - starts server that use unencryped private key
- `server_nopass_client_auth.sh` - starts server that use unencryped private key, and require client authentication
- `server_pass.sh` - starts server that use encrypted/password protected private key
- `server_11.sh` - starts server with maximum TLS version set to 1.1
- `client_nopass.sh` - executes dgraphloader configured to use unencrypted privae key
- `client_pass.sh` - executes dgraphloader configured to use encrypted/password protected private key
- `client_nocert.sh` - executes dgraphloader without configured client certificate
- `client_12.sh` - executes dgraphloader with minimum TLS version set to 1.2

## Notes
Go x509 package supports only encrypted private keys conaining "DEK-Info". By default, openssl doesn't include it in generated keys. Fortunately, if encryption method is explicitly set in the command line, openssl adds "DEK-Info" header.

`server_pass.sh` should be used with `client_pass.sh`. This enable testing of `tls.server_name` configuration option. Mixing `_pass` and `_nopass` client/server shows that server name is verified by the client.

For testing purposes, DNS names for server1.dgraph.io and server2.dgraph.io has to be resolvable. Editing /etc/hosts is the simplest way to achieve this. 
