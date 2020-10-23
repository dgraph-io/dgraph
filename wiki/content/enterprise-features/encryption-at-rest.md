+++
date = "2017-03-20T22:25:17+11:00"
title = "Encryption at Rest"
weight = 3
[menu.main]
    parent = "enterprise-features"
+++

{{% notice "note" %}}
This feature was introduced in [v1.1.1](https://github.com/dgraph-io/dgraph/releases/tag/v1.1.1).
For migrating unencrypted data to a new Dgraph cluster with encryption enabled, you need to
[export the database](https://dgraph.io/docs/deploy/dgraph-administration/#exporting-database) and [fast data load](https://dgraph.io/docs/deploy/#fast-data-loading),
preferably using the [bulk loader](https://dgraph.io/docs/deploy/#bulk-loader).
{{% /notice %}}

Encryption at rest refers to the encryption of data that is stored physically in any
digital form. It ensures that sensitive data on disks is not readable by any user
or application without a valid key that is required for decryption. Dgraph provides
encryption at rest as an enterprise feature. If encryption is enabled, Dgraph uses
[Advanced Encryption Standard (AES)](https://en.wikipedia.org/wiki/Advanced_Encryption_Standard)
algorithm to encrypt the data and secure it.

Prior to v20.07.0, the encryption key file must be present on the local file system.
Starting with [v20.07.0] (https://github.com/dgraph-io/dgraph/releases/tag/v20.07.0),
we have added support for encryption keys sitting on Vault servers. This allows an alternate
way to configure the encryption keys needed for encrypting the data at rest.

## Set up Encryption

To enable encryption, we need to pass a file that stores the data encryption key with the option
`--encryption_key_file`. The key size must be 16, 24, or 32 bytes long, and the key size determines
the corresponding block size for AES encryption ,i.e. AES-128, AES-192, and AES-256, respectively.

You can use the following command to create the encryption key file (set _count_ to the
desired key size):

```
dd if=/dev/random bs=1 count=32 of=enc_key_file
```

Alternatively, you can use the `--vault_*` options to enable encrption as explained below.

## Turn on Encryption

Here is an example that starts one Zero server and one Alpha server with the encryption feature turned on:

```bash
dgraph zero --my=localhost:5080 --replicas 1 --idx 1
dgraph alpha --encryption_key_file ./enc_key_file --my=localhost:7080 --zero=localhost:5080
```

If multiple Alpha nodes are part of the cluster, you will need to pass the `--encryption_key_file` option to
each of the Alphas.

Once an Alpha has encryption enabled, the encryption key must be provided in order to start the Alpha server.
If the Alpha server restarts, the `--encryption_key_file` option must be set along with the key in order to
restart successfully.

Alternatively, for encryption keys sitting on Vault server, here is an example. To use Vault, there are some pre-requisites.
1. Vault Server URL of the form `http://fqdn[ip]:port`. This will be used for the options `--vault_addr`.
2. Vault Server must be configued with an approle auth. A `secret-id` and `role-id` must be generated and copied over to local files. This will be needed for the options `--vault_secretid_file` and `vault_roleid_file`.
3. Vault Server must instantiate a KV store containing a K/V for Dgraph. The `--vault_field` option must be the KV-v1 or KV-v2 format. The vaule of this key is the encryption key that Dgraph will use. This key must be 16,24 or 32 bytes as explained above.

Next, here is an example of using Dgraph with a Vault server that holds the encryption key.
```bash
dgraph zero --my=localhost:5080 --replicas 1 --idx 1
dgraph alpha --vault_addr https://localhost:8200 --vault_roleid_file ./roleid --vault_secretid_file ./secretid --vault_field enc_key_name --my=localhost:7080 --zero=localhost:5080
```

If multiple Alpha nodes are part of the cluster, you will need to pass the `--encryption_key_file` option or the `--vault_*` options to
each of the Alphas.

Once an Alpha has encryption enabled, the encryption key must be provided in order to start the Alpha server.
If the Alpha server restarts, the `--encryption_key_file` or the `--vault_*` option must be set along with the key in order to
restart successfully.

## Turn off Encryption

If you wish to turn off encryption from an existing Alpha, then you can export your data and import it (using [live loader](https://dgraph.io/docs/deploy/fast-data-loading/#live-loader) into a new Dgraph instance without encryption enabled. You will have to use the `--encryption_key_file` flag while importing.

```
dgraph live -f <path-to-gzipped-RDF-or-JSON-file> -s <path-to-schema> --encryption_key_file <path-to-enc_key_file> -a <dgraph-alpha-address:grpc_port> -z <dgraph-zero-address:grpc_port>
```

## Change Encryption Key

The master encryption key set by the `--encryption_key_file` option (or one used in Vault KV store) does not change automatically. The master
encryption key encrypts underlying *data keys* which are changed on a regular basis automatically (more info
about this is covered on the encryption-at-rest [blog][encblog] post).

[encblog]: https://dgraph.io/blog/post/encryption-at-rest-dgraph-badger#one-key-to-rule-them-all-many-keys-to-find-them

Changing the existing key to a new one is called key rotation. You can rotate the master encryption key by
using the `badger rotate` command on both p and w directories for each Alpha. To maintain availability in HA
cluster configurations, you can do this rotate the key one Alpha at a time in a rolling manner.

You'll need both the current key and the new key in two different files. Specify the directory you
rotate ("p" or "w") for the `--dir` flag, the old key for the `--old-key-path` flag, and the new key with the
`--new-key-path` flag.

```
badger rotate --dir p --old-key-path enc_key_file --new-key-path new_enc_key_file
badger rotate --dir w --old-key-path enc_key_file --new-key-path new_enc_key_file
```

Then, you can start Alpha with the `new_enc_key_file` key file to use the new key.
