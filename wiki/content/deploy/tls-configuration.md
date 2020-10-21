+++
date = "2017-03-20T22:25:17+11:00"
title = "TLS Configuration"
weight = 10
[menu.main]
    parent = "deploy"
+++

{{% notice "note" %}}
This section refers to the `dgraph cert` command which was introduced in v1.0.9. For previous releases, see the previous [TLS configuration documentation](https://dgraph.io/docs/v1.0.7/deploy/#tls-configuration).
{{% /notice %}}


Connections between client and server can be secured with TLS. Password protected private keys are **not supported**.

{{% notice "tip" %}}If you're generating encrypted private keys with `openssl`, be sure to specify encryption algorithm explicitly (like `-aes256`). This will force `openssl` to include `DEK-Info` header in private key, which is required to decrypt the key by Dgraph. When default encryption is used, `openssl` doesn't write that header and key can't be decrypted.{{% /notice %}}

## Dgraph Certificate Management Tool

The `dgraph cert` program creates and manages CA-signed certificates and private keys using a generated Dgraph Root CA. There are three types of these pairs:
1. The Root CA certificate/key pair: It is used to sign and verify node and client certificates, if the root CA certificate is changed then you must regenerate all the certificates.
2. Node certificate/key pair: It is shared by the alpha nodes for accepting TLS connections.
3. Client certificate/key pair: It is used by the clients (like live loader, ratel etc) to communicate with the alpha server.

```sh
# To see the available flags.
$ dgraph cert --help

# Create Dgraph Root CA, used to sign all other certificates.
$ dgraph cert

# Create node certificate and private key
$ dgraph cert -n localhost

# Create client certificate and private key for mTLS (mutual TLS)
$ dgraph cert -c dgraphuser

# Combine all in one command
$ dgraph cert -n localhost -c dgraphuser

# List all your certificates and keys
$ dgraph cert ls
```

The default location where the _cert_ command stores certificates (and keys) is `tls` under the Dgraph working directory. The default dir path can be overridden using the `--dir` option. For example

```sh
$ dgraph cert --dir ~/mycerts
```

### File naming conventions

The following file naming conventions are used by Dgraph for proper TLS setup.

| File name | Description | Use |
|-----------|-------------|-------|
| ca.crt | Dgraph Root CA certificate | Verify all certificates |
| ca.key | Dgraph CA private key | Validate CA certificate |
| node.crt | Dgraph node certificate | Shared by all nodes for accepting TLS connections |
| node.key | Dgraph node private key | Validate node certificate |
| client._name_.crt | Dgraph client certificate | Authenticate a client _name_ |
| client._name_.key | Dgraph client private key | Validate _name_ client certificate |

For client authentication, each client must have their own certificate and key. These are then used to connect to the Dgraph node(s).

The node certificate `node.crt` can support multiple node names using multiple host names and/or IP address. Just separate the names with commas when generating the certificate.

```sh
$ dgraph cert -n localhost,104.25.165.23,dgraph.io,2400:cb00:2048:1::6819:a417
```

{{% notice "tip" %}}You must delete the old node cert and key before you can generate a new pair.{{% /notice %}}

{{% notice "note" %}}When using host names for node certificates, including _localhost_, your clients must connect to the matching host name -- such as _localhost_ not 127.0.0.1. If you need to use IP addresses, then add them to the node certificate.{{% /notice %}}

### Certificate inspection

The command `dgraph cert ls` lists all certificates and keys in the `--dir` directory (default 'tls'), along with details to inspect and validate cert/key pairs.

Example of command output:

```sh
-rw-r--r-- ca.crt - Dgraph Root CA certificate
        Issuer: Dgraph Labs, Inc.
           S/N: 043c4d8fdd347f06
    Expiration: 02 Apr 29 16:56 UTC
SHA-256 Digest: 4A2B0F0F 716BF5B6 C603E01A 6229D681 0B2AFDC5 CADF5A0D 17D59299 116119E5

-r-------- ca.key - Dgraph Root CA key
SHA-256 Digest: 4A2B0F0F 716BF5B6 C603E01A 6229D681 0B2AFDC5 CADF5A0D 17D59299 116119E5

-rw-r--r-- client.admin.crt - Dgraph client certificate: admin
        Issuer: Dgraph Labs, Inc.
     CA Verify: PASSED
           S/N: 297e4cb4f97c71f9
    Expiration: 03 Apr 24 17:29 UTC
SHA-256 Digest: D23EFB61 DE03C735 EB07B318 DB70D471 D3FE8556 B15D084C 62675857 788DF26C

-rw------- client.admin.key - Dgraph Client key
SHA-256 Digest: D23EFB61 DE03C735 EB07B318 DB70D471 D3FE8556 B15D084C 62675857 788DF26C

-rw-r--r-- node.crt - Dgraph Node certificate
        Issuer: Dgraph Labs, Inc.
     CA Verify: PASSED
           S/N: 795ff0e0146fdb2d
    Expiration: 03 Apr 24 17:00 UTC
         Hosts: 104.25.165.23, 2400:cb00:2048:1::6819:a417, localhost, dgraph.io
SHA-256 Digest: 7E243ED5 3286AE71 B9B4E26C 5B2293DA D3E7F336 1B1AFFA7 885E8767 B1A84D28

-rw------- node.key - Dgraph Node key
SHA-256 Digest: 7E243ED5 3286AE71 B9B4E26C 5B2293DA D3E7F336 1B1AFFA7 885E8767 B1A84D28
```

Important points:

* The cert/key pairs should always have matching SHA-256 digests. Otherwise, the cert(s) must be
  regenerated. If the Root CA pair differ, all cert/key must be regenerated; the flag `--force`
  can help.
* All certificates must pass Dgraph CA verification.
* All key files should have the least access permissions, especially the `ca.key`, but be readable.
* Key files won't be overwritten if they have limited access, even with `--force`.
* Node certificates are only valid for the hosts listed.
* Client certificates are only valid for the named client/user.

## TLS Options

The following configuration options are available for Alpha:

* `--tls_dir string` - TLS dir path; this enables TLS connections (usually 'tls').
* `--tls_use_system_ca` - Include System CA with Dgraph Root CA.
* `--tls_client_auth string` - TLS client authentication used to validate client connection. See [Client Authentication Options](#client-authentication-options) for details.

Dgraph Live Loader can be configured with the following options:

* `--tls_cacert string` - Dgraph Root CA, such as `./tls/ca.crt`
* `--tls_use_system_ca` - Include System CA with Dgraph Root CA.
* `--tls_cert` - User cert file provided by the client to Alpha
* `--tls_key` - User private key file provided by the client to Alpha
* `--tls_server_name string` - Server name, used for validating the server's TLS host name.


### Using TLS without Client Authentication

For TLS without client authentication, you can configure certificates and run Alpha server using the following:

```sh
# First, create rootca and node certificates and private keys
$ dgraph cert -n localhost
# Default use for enabling TLS server (after generating certificates and private keys)
$ dgraph alpha --tls_dir tls
```

You can then run Dgraph live loader using the following:

```sh
# Now, connect to server using TLS
$ dgraph live --tls_cacert ./tls/ca.crt --tls_server_name "localhost" -s 21million.schema -f 21million.rdf.gz
```

### Using TLS with Client Authentication

If you do require Client Authentication (Mutual TLS), you can configure certificates and run Alpha server using the following:

```sh
# First, create a rootca, node, and client certificates and private keys
$ dgraph cert -n localhost -c dgraphuser
# Default use for enabling TLS server with client authentication (after generating certificates and private keys)
$ dgraph alpha --tls_dir tls --tls_client_auth="REQUIREANDVERIFY"
```

You can then run Dgraph live loader using the following:

```sh
# Now, connect to server using mTLS (mutual TLS)
$ dgraph live \
   --tls_cacert ./tls/ca.crt \
   --tls_cert ./tls/client.dgraphuser.crt \
   --tls_key ./tls/client.dgraphuser.key \
   --tls_server_name "localhost" \
   -s 21million.schema \
   -f 21million.rdf.gz
```

### Client Authentication Options

The server will always **request** Client Authentication.  There are four different values for the `--tls_client_auth` option that change the security policy of the client certificate.

| Value              | Client Cert/Key | Client Certificate Verified |
|--------------------|-----------------|--------------------|
| `REQUEST`          | optional        | Client certificate is not VERIFIED if provided. (least secure) |
| `REQUIREANY`       | required        | Client certificate is never VERIFIED |
| `VERIFYIFGIVEN`    | optional        | Client certificate is VERIFIED if provided (default) |
| `REQUIREANDVERIFY` | required        | Client certificate is always VERIFIED (most secure) |

{{% notice "note" %}} `REQUIREANDVERIFY` is the most secure but also the most difficult to configure for remote clients. When using this value, the value of `--tls_server_name` is matched against the certificate SANs values and the connection host.{{% /notice %}}

## Using Ratel UI with Client authentication

Ratel UI (and any other JavaScript clients built on top of `dgraph-js-http`)
connect to Dgraph servers via HTTP, when TLS is enabled servers begin to expect
HTTPS requests only.

If you haven't already created the CA certificate and the node certificate for alpha servers from the earlier instructions (see [Dgraph Certificate Management Tool](#dgraph-certificate-management-tool)), the first step would be to generate these certificates, it can be done by the following command:
```sh
# Create rootCA and node certificates/keys
$ dgraph cert -n localhost
```

If `--tls_client_auth` option in dgraph alpha is set to `REQUEST` or `VERIFYIFGIVEN` (default), then client certificate is not mandatory. The steps after generating CA/node certificate are as follows:

### Step 1. Install Dgraph Root CA into System CA
##### Linux (Debian/Ubuntu)
```sh
# Copy the generated CA to the ca-certificates directory
$ cp /path/to/ca.crt /usr/local/share/ca-certificates/ca.crt
# Update the CA store
$ sudo update-ca-certificates`
```
##### Mac OS X
```sh
$ sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain /path/to/ca.crt
```
##### Windows
```sh
$ certutil -addstore -f "ROOT" /path/to/ca.crt
```

### Step 2. Install Dgraph Root CA into Web Browsers Trusted CA List

##### Firefox

* Goto Preferences -> Prvacy & Security -> View Certificates -> Authorities
* Click on Import and import the `ca.crt`

##### Chrome

* Goto Settings -> Privacy and Security -> Security -> Manage Certificates -> Authorities
* Click on Import and import the `ca.crt`

### Step 3. Point ratel to the `https://` endpoint of alpha server.

* Change the Dgraph's alpha server address to `https://` instead of `http://`, for example `https://localhost:8080`.

For `REQUIREANY` and `REQUIREANDVERIFY` as `--tls_client_auth` option of alpha, you need to follow the steps above and you
also need to install client certificate on your browser:

1. Generate a client certificate: `dgraph cert -c laptopuser`.
2. Convert it to a `.p12` file:
   ```sh
   openssl pkcs12 -export \
      -out laptopuser.p12 \
      -in tls/client.laptopuser.crt \
      -inkey tls/client.laptopuser.key
   ```
   Use any password you like for export, it is used to encrypt the p12 file.

3. Import the client certificate to your browser. It can be done in chrome as follows:
   * Goto Settings -> Privacy and Security -> Security -> Manage Certificates -> Your Certificates
   * Click on Import and import the `laptopuser.p12`. For mac OS, this process returns back to KeyChain, and under the area "My Certificates" select `laptopuser.p12`.

{{% notice "note" %}}
Under macOS you can alternatively import the `.p12` file via command line by `security import ./laptopuser.p12 -P secretPassword`.
{{% /notice %}}
{{% notice "note" %}}
Mutual TLS may not work in Firefox because Firefox is unable to send privately-signed client certificates, this issue is filed [here](https://bugzilla.mozilla.org/show_bug.cgi?id=1662607).
{{% /notice %}}


Next time you use Ratel to connect to an alpha with Client authentication
enabled the browser will prompt you for a client certificate to use. Select the client's
certificate you've imported in the step above and queries/mutations will
succeed.

## Using Curl with Client authentication

When TLS is enabled, `curl` requests to Dgraph will need some specific options to work.  For instance (for an export request):

```
curl --silent https://localhost:8080/admin/export
```

If you are using `curl` with [Client Authentication](#client-authentication-options) set to `REQUIREANY` or `REQUIREANDVERIFY`, you will need to provide the client certificate and private key.  For instance (for an export request):

```
curl --silent --cacert ./tls/ca.crt --cert ./tls/client.dgraphuser.crt --key ./tls/client.dgraphuser.key https://localhost:8080/admin/export
```

Refer to the `curl` documentation for further information on its TLS options.

## Access Data Using a Client

Some examples of connecting via a [Client](/clients) when TLS is in use can be found below:

- [dgraph4j](https://github.com/dgraph-io/dgraph4j#creating-a-secure-client-using-tls)
- [dgraph-js](https://github.com/dgraph-io/dgraph-js/tree/master/examples/tls)
- [dgo](https://github.com/dgraph-io/dgraph/blob/master/tlstest/acl/acl_over_tls_test.go)
- [pydgraph](https://github.com/dgraph-io/pydgraph/tree/master/examples/tls)

## Troubleshooting Ratel's Client authentication

If you are getting errors in Ratel when server's TLS is enabled try opening
your alpha URL as a webpage.

Assuming you are running Dgraph on your local machine, opening
`https://localhost:8080/` in browser should produce a message `Dgraph browser is available for running separately using the dgraph-ratel binary`.

In case you are getting a connection error, try not passing the
`--tls_client_auth` flag when starting an alpha. If you are still getting an
error, check that your hostname is correct and the port is open; then make sure
that "Dgraph Root CA" certificate is installed and trusted correctly.

After that, if things work without `--tls_client_auth` but stop working when
`REQUIREANY` and `REQUIREANDVERIFY` is set make sure the `.p12` file is
installed correctly.