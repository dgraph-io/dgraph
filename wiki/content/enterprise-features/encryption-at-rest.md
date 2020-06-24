+++
date = "2017-03-20T22:25:17+11:00"
title = "Encryption at Rest"
[menu.main]
    parent = "enterprise-features"
    weight = 3
+++

{{% notice "note" %}}
This feature was introduced in [v1.1.1](https://github.com/dgraph-io/dgraph/releases/tag/v1.1.1).
For migrating unencrypted data to a new Dgraph cluster with encryption enabled, you need to
[export the database](https://dgraph.io/docs/deploy/#exporting-database) and [fast data load](https://dgraph.io/docs/deploy/#fast-data-loading),
preferably using the [bulk loader](https://dgraph.io/docs/deploy/#bulk-loader).
{{% /notice %}}

Encryption at rest refers to the encryption of data that is stored physically in any
digital form. It ensures that sensitive data on disks is not readable by any user
or application without a valid key that is required for decryption. Dgraph provides
encryption at rest as an enterprise feature. If encryption is enabled, Dgraph uses
[Advanced Encryption Standard (AES)](https://en.wikipedia.org/wiki/Advanced_Encryption_Standard)
algorithm to encrypt the data and secure it.

## Set up Encryption

To enable encryption, we need to pass a file that stores the data encryption key with the option
`--encryption_key_file`. The key size must be 16, 24, or 32 bytes long, and the key size determines
the corresponding block size for AES encryption ,i.e. AES-128, AES-192, and AES-256, respectively.

You can use the following command to create the encryption key file (set _count_ to the
desired key size):

```
dd if=/dev/random bs=1 count=32 of=enc_key_file
```

## Turn on Encryption

Here is an example that starts one Zero server and one Alpha server with the encryption feature turned on:

```bash
dgraph zero --my=localhost:5080 --replicas 1 --idx 1
dgraph alpha --encryption_key_file ./enc_key_file --my=localhost:7080 --lru_mb=1024 --zero=localhost:5080
```

If multiple Alpha nodes are part of the cluster, you will need to pass the `--encryption_key_file` option to
each of the Alphas.

Once an Alpha has encryption enabled, the encryption key must be provided in order to start the Alpha server.
If the Alpha server restarts, the `--encryption_key_file` option must be set along with the key in order to
restart successfully.

## Turn off Encryption

If you wish to turn off encryption from an existing Alpha, then you can export your data and import it
into a new Dgraph instance without encryption enabled.

## Change Encryption Key

The master encryption key set by the `--encryption_key_file` option does not change automatically. The master
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

