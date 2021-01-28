+++
title = "Data Decryption"
weight = 18
[menu.main]
    parent = "deploy"
+++

You might need to decrypt data from an encrypted Dgraph cluster for a variety of
reasons, including:

* Migration of data from an encrypted cluster to a non-encrypted cluster
* Changing your data or schema by directly editing an RDF file or schema file

To support these scenarios, Dgraph includes a `decrypt`
command that decrypts encrypted RDF and schema files. To learn how to export RDF
and schema files from Dgraph, see:
[Dgraph Administration: Export database](/deploy/dgraph-administration/#exporting-database).

The `decrypt` command supports a variety of symmetric key lengths, which
determine the AES cypher used for encryption and decryption, as follows:


| Symmetric key length | AES encryption cypher |
|----------------------|-----------------------|
| 128 bits (16-bytes)  |  AES-128              |
| 192 bits (24-bytes)  |  AES-192              |
| 256 bits (32-bytes)  |  AES-256              |


The `decrypt` command also supports the use of
[Vault](https://www.vaultproject.io/) to store secrets, including support for
Vault's
[AppRole authentication](https://www.vaultproject.io/docs/auth/approle.html).

## Decryption options

The following decryption options (or *flags*) are available for the `decrypt` command:

| Option                      | Notes                                                                |
|-----------------------------|----------------------------------------------------------------------|
|`encryption_key_file`        | Encryption key filename                                              |
|`-f`, `--file`               | Path and filename for the encrypted RDF or schema **.gz** file       |
|`-h`, `--help`               | Help for the `decrypt` command                                       |
|`-o`, `--out`                | Path and filename for the decrypted .gz file that `decrypt` creates  |
|`--vault_addr`               | Vault server address, in **http://&lt;*ip-address*&gt;:&lt;*port*&gt;** format (default: `http://localhost:8200` ) |
|`--vault_field`              | Name of the Vault server's key/value store field that holds the Base64 encryption key (default `enc_key`) |
|`--vault_format`             | Vault server field format; can be `raw` or `base64` (default: `base64`) |
|`--vault_path`               | Vault server key/value store path (default: `secret/data/dgraph`)       |
|`--vault_roleid_file`        | File containing the Vault `role-id` used for AppRole authentication     |
|`--vault_secretid_file`      |  File containing the Vault `secret-id` used for AppRole authentication  |

## Data decryption examples 

For example, you could use the following command with an encrypted RDF file
(**encrypted.rdf.gz**) and an encryption key file (**enc_key_file**), to
create a decrypted RDF file:

```
dgraph decrypt -f encrypted.rdf.gz --encryption_key_file enc-key-file -o decrypted_rdf.gz
```

You can use similar syntax to create a decrypted schema file:

```
dgraph decrypt -f encrypted.schema.gz --encryption_key_file enc-key-file -o decrypted_schema.gz
```


