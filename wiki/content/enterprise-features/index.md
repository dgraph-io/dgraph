+++
date = "2017-03-20T19:35:35+11:00"
title = "Enterprise Features"
+++

Dgraph enterprise features are proprietary licensed under the [Dgraph Community
License][dcl]. All Dgraph releases contain proprietary code for enterprise features.
Enabling these features requires an enterprise contract from
[contact@dgraph.io](mailto:contact@dgraph.io) or the [discuss
forum](https://discuss.dgraph.io).

**Dgraph enterprise features are enabled by default for 30 days in a new cluster**.
After the trial period of thirty (30) days, the cluster must obtain a license from Dgraph to
continue using the enterprise features released in the proprietary code.

{{% notice "note" %}}
At the conclusion of your 30-day trial period if a license has not been applied to the cluster,
access to the enterprise features will be suspended. The cluster will continue to operate without
enterprise features.
{{% /notice %}}

When you have an enterprise license key, the license can be applied to the cluster by including it
as the body of a POST request and calling `/enterpriseLicense` HTTP endpoint on any Zero server.

```sh
curl -X POST localhost:6080/enterpriseLicense --upload-file ./licensekey.txt
```

It can also be applied by passing the path to the enterprise license file (using the flag
`--enterprise_license`) to the `dgraph zero` command used to start the server. The second option is
useful when the process needs to be automated.

```sh
dgraph zero --enterprise_license ./licensekey.txt
```

[dcl]: https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt

## Binary Backups

{{% notice "note" %}}
This feature was introduced in [v1.1.0](https://github.com/dgraph-io/dgraph/releases/tag/v1.1.0).
{{% /notice %}}

Binary backups are full backups of Dgraph that are backed up directly to cloud
storage such as Amazon S3 or any Minio storage backend. Backups can also be
saved to an on-premise network file system shared by all alpha instances. These
backups can be used to restore a new Dgraph cluster to the previous state from
the backup. Unlike [exports]({{< relref "deploy/index.md#exporting-database" >}}),
binary backups are Dgraph-specific and can be used to restore a cluster quickly.

### Configure Backup

Backup is only enabled when a valid license file is supplied to a Zero server OR within the thirty
(30) day trial period, no exceptions.

#### Configure Amazon S3 Credentials

To backup to Amazon S3, the Alpha must have the following AWS credentials set
via environment variables:

 Environment Variable                        | Description
 --------------------                        | -----------
 `AWS_ACCESS_KEY_ID` or `AWS_ACCESS_KEY`     | AWS access key with permissions to write to the destination bucket.
 `AWS_SECRET_ACCESS_KEY` or `AWS_SECRET_KEY` | AWS access key with permissions to write to the destination bucket.
 `AWS_SESSION_TOKEN`                         | AWS session token (if required).

#### Configure Minio Credentials

To backup to Minio, the Alpha must have the following Minio credentials set via
environment variables:

 Environment Variable                        | Description
 --------------------                        | -----------
 `MINIO_ACCESS_KEY`                          | Minio access key with permissions to write to the destination bucket.
 `MINIO_SECRET_KEY`                          | Minio secret key with permissions to write to the destination bucket.

### Create a Backup

To create a backup, make an HTTP POST request to `/admin` to a Dgraph
Alpha HTTP address and port (default, "localhost:8080"). Like with all `/admin`
endpoints, this is only accessible on the same machine as the Alpha unless
[whitelisted for admin operations]({{< relref "deploy/index.md#whitelisting-admin-operations" >}}).
Execute the following mutation on /admin endpoint using any GraphQL compatible client like Insomnia, GraphQL Playground or GraphiQL.

#### Backup to Amazon S3

```graphql
mutation {
  backup(input: {destination: "s3://s3.us-west-2.amazonaws.com/<bucketname>"}) {
    response {
      message
      code
    }
  }
}
```

#### Backup to Minio

```graphql
mutation {
  backup(input: {destination: "minio://127.0.0.1:9000/<bucketname>"}) {
    response {
      message
      code
    }
  }
}
```


#### Backup to Minio

#### Backup to Google Cloud Storage via Minio Gateway

1. [Create a Service Account key](https://github.com/minio/minio/blob/master/docs/gateway/gcs.md#11-create-a-service-account-ey-for-gcs-and-get-the-credentials-file) for GCS and get the Credentials File
2. Run MinIO GCS Gateway Using Docker
```
docker run -p 9000:9000 --name gcs-s3 \
 -v /path/to/credentials.json:/credentials.json \
 -e "GOOGLE_APPLICATION_CREDENTIALS=/credentials.json" \
 -e "MINIO_ACCESS_KEY=minioaccountname" \
 -e "MINIO_SECRET_KEY=minioaccountkey" \
 minio/minio gateway gcs yourprojectid
```
3. Run MinIO GCS Gateway Using the MinIO Binary
```
export GOOGLE_APPLICATION_CREDENTIALS=/path/to/credentials.json
export MINIO_ACCESS_KEY=minioaccesskey
export MINIO_SECRET_KEY=miniosecretkey
minio gateway gcs yourprojectid
```
#### Test Using MinIO Browser
MinIO Gateway comes with an embedded web-based object browser that outputs content to http://127.0.0.1:9000. To test that MinIO Gateway is running, open a web browser, navigate to http://127.0.0.1:9000, and ensure that the object browser is displayed.
![](https://github.com/minio/minio/blob/master/docs/screenshots/minio-browser-gateway.png?raw=true)

#### Test Using MinIO Client

MinIO Client is a command-line tool called mc that provides UNIX-like commands for interacting with the server (e.g. ls, cat, cp, mirror, diff, find, etc.). mc supports file systems and Amazon S3-compatible cloud storage services (AWS Signature v2 and v4).
MinIO Client is a command-line tool called mc that provides UNIX-like commands for interacting with the server (e.g. ls, cat, cp, diff, find, etc.). mc supports file systems and Amazon S3-compatible cloud storage services (AWS Signature v2 and v4).

1. Configure the Gateway using MinIO Client

Use the following command to configure the gateway:
```
mc config host add mygcs http://gateway-ip:9000 minioaccesskey miniosecretkey
```

2. List Containers on GCS

Use the following command to list the containers on GCS:

```
mc ls mygcs
```
A response similar to this one should be displayed:

```
[2017-02-22 01:50:43 PST]     0B ferenginar/
[2017-02-26 21:43:51 PST]     0B my-container/
[2017-02-26 22:10:11 PST]     0B test-container1/
```

#### Disabling HTTPS for S3 and Minio backups

By default, Dgraph assumes the destination bucket is using HTTPS. If that is not
the case, the backup will fail. To send a backup to a bucket using HTTP
(insecure), set the query parameter `secure=false` with the destination
endpoint in the `destination` field:

```graphql
mutation {
  backup(input: {destination: "minio://127.0.0.1:9000/<bucketname>?secure=false"}) {
    response {
      message
      code
    }
  }
}
```

#### Overriding Credentials

The `accessKey`, `secretKey`, and `sessionToken` parameters can be used to
override the default credentials. Please note that unless HTTPS is used, the
credentials will be transmitted in plain text so use these parameters with
discretion. The environment variables should be used by default but these
options are there to allow for greater flexibility.

The `anonymous` parameter can be set to "true" to a allow backing up to S3 or
Minio bucket that requires no credentials (i.e a public bucket).

#### Backup to NFS

```graphql
mutation {
  backup(input: {destination: "/path/to/local/directory"}) {
    response {
      message
      code
    }
  }
}
```

A local filesystem will work only if all the Alphas have access to it (e.g all
the Alphas are running on the same filesystems as a normal process, not a Docker
container). However, a NFS is recommended so that backups work seamlessly across
multiple machines and/or containers.

#### Forcing a Full Backup

By default, an incremental backup will be created if there's another full backup
in the specified location. To create a full backup, set the `forceFull` field
to `true` in the mutation. Each series of backups can be
identified by a unique ID and each backup in the series is assigned a
monotonically increasing number. The following section contains more details on
how to restore a backup series.
```graphql
mutation {
  backup(input: {destination: "/path/to/local/directory", forceFull: true}) {
    response {
      message
      code
    }
  }
}
```

### Encrypted Backups

Encrypted backups are a Enterprise feature that are available from v20.03.1 and v1.2.3 and allow you to encrypt your backups and restore them. This documentation describes how to implement encryption into your binary backups

### New flag “Encrypted” in manifest.json

A new flag “Encrypted” is added to the `manifest.json`. This flag indicates if the corresponding binary backup is encrypted or not. To be backward compatible, if this flag is absent, it is presumed that the corresponding backup is not encrypted.

For a series of full and incremental backups, per the current design, we don't allow mixing of encrypted and unencrypted backups. As a result, all full and incremental backups in a series must either be encrypted fully or not at all. This flag helps with checking this restriction.

### AES And Chaining with Gzip

If encryption is turned on an alpha, then we use the configured encryption key. The key size (16, 24, 32 bytes) determines AES-128/192/256 cipher chosen. We use the AES CTR mode. Currently, the binary backup is already gzipped. With encryption, we will encrypt the gzipped data.

During **backup**: the 16 bytes IV is prepended to the Cipher-text data after encryption.

### Backup

Backup is an online tool, meaning it is available when alpha is running. For encrypted backups, the alpha must be configured with the “encryption_key_file”.

{{% notice "note" %}}
encryption_key_file was used for encryption-at-rest and will now also be used for encrypted backups.
{{% /notice %}}

The restore utility is a standalone tool today. Hence, a new flag “keyfile” is added to the restore utility so it can decrypt the backup. This keyfile must be the same key that was used for encryption during backup.

### Restore from Backup

The `dgraph restore` command restores the postings directory from a previously
created backup to a directory in the local filesystem. Restore is intended to
restore a backup to a new Dgraph cluster not a currently live one. During a
restore, a new Dgraph Zero may be running to fully restore the backup state.

The `--location` (`-l`) flag specifies a source URI with Dgraph backup objects.
This URI supports all the schemes used for backup.

The `--postings` (`-p`) flag sets the directory to which the restored posting
directories will be saved. This directory will contain a posting directory for
each group in the restored backup.

The `--zero` (`-z`) flag specifies a Dgraph Zero address to update the start
timestamp and UID lease using the restored version. If no zero address is
passed, the command will complain unless you set the value of the
`--force_zero` flag to false. If do not pass a zero value to this command,
the timestamp and UID lease must be manually updated through Zero's HTTP
'assign' endpoint using the values printed near the end of the command's output.

The `--backup_id` optional flag specifies the ID of the backup series to
restore. A backup series consists of a full backup and all the incremental
backups built on top of it. Each time a new full backup is created, a new backup
series with a different ID is started. The backup series ID is stored in each
`manifest.json` file stored in every backup folder.

The `--encryption_key_file` flag is required if you took the backup in an
encrypted cluster and should point to the location of the same key used to
run the cluster.

The restore feature will create a cluster with as many groups as the original
cluster had at the time of the last backup. For each group, `dgraph restore`
creates a posting directory `p<N>` corresponding to the backup group ID. For
example, a backup for Alpha group 2 would have the name `.../r32-g2.backup`
and would be loaded to posting directory `p2`.

After running the restore command, the directories inside the `postings`
directory need to be manually copied over to the machines/containers running the
alphas before running the `dgraph alpha` command. For example, in a database
cluster with two Alpha groups and one replica each, `p1` needs to be moved to
the location of the first Alpha and `p2` needs to be moved to the location of
the second Alpha.

By default, Dgraph will look for a posting directory with the name `p`, so make
sure to rename the directories after moving them. You can also use the `-p`
option of the `dgraph alpha` command to specify a different path from the default.

#### Restore from Amazon S3
```sh
$ dgraph restore -p /var/db/dgraph -l s3://s3.us-west-2.amazonaws.com/<bucketname>
```

#### Restore from Minio
```sh
$ dgraph restore -p /var/db/dgraph -l minio://127.0.0.1:9000/<bucketname>
```

#### Restore from Local Directory or NFS
```sh
$ dgraph restore -p /var/db/dgraph -l /var/backups/dgraph
```

#### Restore and Update Timestamp

Specify the Zero address and port for the new cluster with `--zero`/`-z` to update the timestamp.
```sh
$ dgraph restore -p /var/db/dgraph -l /var/backups/dgraph -z localhost:5080
```

### Online Restore

Starting in version 20.07, restoring to a live cluster is supported. Instead of
running the command line tool and copying the `p` directories around, users can
now point the cluster to the backup location and the cluster will read the backup,
write the data, and set the cluster so that it can read the data.

Some things to note about online restores are:

- Existing data is dropped. This is the equivalent of sending a DropAll
  operation to the cluster.
- Draining mode is enabled during the duration of the restore. This means
  queries and mutations are disabled. Draining mode is turned off when the
  restore completes.
- The operation is asynchronous and the request responds immediately after
  scheduling the restore. The response includes a unique restore ID that can be
  used to query the status of the operation.
- This operation should only be done when the cluster is new or when an existing
  cluster has been taken offline for maintenance.
- The cluster needs to have the same number of groups as the cluster where the
  backup was created.
- The predicates will be forcibly moved around groups to ensure the state of the
  cluster after restore matches what is indicated by the backup manifest. The
  process does not require existing data to be moved around since it will be
  deleted anyways.

### Basic operation

A restore can be started via the GraphQL admin endpoint (i.e. `/admin`) of any
alpha in the cluster. This is the same endpoint to which backup requests are
sent. The basic call looks like this.

```graphql
mutation restore() {
	restore(input: {location: "your-backup-location", backupId: "your-backup-id") {
		code
		message
		restoreId
	}
}
```

If the restore was successfully scheduled, the response will include the restore ID that
can be used to query for the status of the restore via the admin endpoint. Below is an
example.

```graphql
query status() {
	restoreStatus(restoreId: 1) {
		status
		errors
	}
}
```

The status can be one of "OK", "ERR", "UNKNOWN", or "IN_PROGRESS". The errors
field contains the errors if the operation failed and it will be empty
otherwise. Also note that the information used to track a restore operation is
local to each alpha. In practical terms, this means that you should send this
request to the same alpha to which the original restore request was sent.

### Online Restore Options

The input parameter can include the fields shown below. Of those, only
`location` is required. Like backups, the authentication info (e.g access key)
is only required if you need to override the defaults that are set via
environment variables (see section on backups for reference).
The fields related to Vault are also only needed if you need to override the
defaults.

- location: Destination for the backup: e.g. Minio or S3 bucket.
- backupId: Backup ID of the backup series to restore. This ID is included in
  the manifest.json file. If missing, it defaults to the latest series.
- encryptionKeyFile: Path to the key file needed to decrypt the backup. This
  file should be accessible by all alphas in the group. The backup will be
  written using the encryption key with which the cluster was started, which
  might be different than this key.
- accessKey: Access key credential for the S3 or minio destination.
- secretKey: Secret key credential for the S3 or minio destination .
- sessionToken: AWS session token, if required.
- anonymous: Set to true to allow backing up to S3 or Minio bucket that requires
  no credentials.
- vaultAddr: Vault server address where the key is stored. This server must be
  accessible by all alphas in the group. Default "http://localhost:8200".
- vaultRoleIDFile: Path to the Vault RoleID file.
- vaultSecretIDFile: Path to the Vault SecretID file.
- vaultPath: Vault kv store path where the key lives. Default
  "secret/data/dgraph".
- vaultField: Vault kv store field whose value is the key. Default "enc_key".
- vaultFormat: Vault kv store field's format. Must be "base64" or "raw". Default
  "base64".

## Access Control Lists

{{% notice "note" %}}
This feature was introduced in [v1.1.0](https://github.com/dgraph-io/dgraph/releases/tag/v1.1.0).
The Dgraph ACL tool is deprecated and would be removed in the next release. ACL changes can be made by using the `/admin` GraphQL endpoint on any Alpha node.
{{% /notice %}}

Access Control List (ACL) provides access protection to your data stored in
Dgraph. When the ACL feature is turned on, a client, e.g. dgo or dgraph4j, must
authenticate with a username and password before executing any transactions, and
is only allowed to access the data permitted by the ACL rules.

This document has two parts: first we will talk about the admin operations
needed for setting up ACL; then we will explain how to use a client to access
the data protected by ACL rules.

### Turn on ACLs

The ACL Feature can be turned on by following these steps

1. Since ACL is an enterprise feature, make sure your use case is covered under
a contract with Dgraph Labs Inc. You can contact us by sending an email to
[contact@dgraph.io](mailto:contact@dgraph.io) or post your request at [our discuss
forum](https://discuss.dgraph.io) to get an enterprise license.

2. Create a plain text file, and store a randomly generated secret key in it. The secret
key is used by Alpha servers to sign JSON Web Tokens (JWT). As you’ve probably guessed,
it’s critical to keep the secret key as a secret. Another requirement for the secret key
is that it must have at least 256-bits, i.e. 32 ASCII characters, as we are using
HMAC-SHA256 as the signing algorithm.

3. Start all the alpha servers in your cluster with the option `--acl_secret_file`, and
make sure they are all using the same secret key file created in Step 2.

Here is an example that starts one zero server and one alpha server with the ACL feature turned on:

```bash
dgraph zero --my=localhost:5080 --replicas 1 --idx 1 --bindall --expose_trace --profile_mode block --block_rate 10 --logtostderr -v=2
dgraph alpha --my=localhost:7080 --lru_mb=1024 --zero=localhost:5080 --logtostderr -v=3 --acl_secret_file ./hmac-secret
```

If you are using docker-compose, a sample cluster can be set up by:

1. `cd $GOPATH/src/github.com/dgraph-io/dgraph/compose/`
2. `make`
3. `./compose -e --acl_secret <path to your hmac secret file>`, after which a `docker-compose.yml` file will be generated.
4. `docker-compose up` to start the cluster using the `docker-compose.yml` generated above.

### Set up ACL Rules

Now that your cluster is running with the ACL feature turned on, you can set up the ACL rules. This can be done using the web UI Ratel or by using a GraphQL tool which fires the mutations. Execute the following mutations using a GraphQL tool like Insomnia, GraphQL Playground or GraphiQL.

A typical workflow is the following:

1. Reset the root password
2. Create a regular user
3. Create a group
4. Assign the user to the group
5. Assign predicate permissions to the group

#### Using GraphQL Admin API
{{% notice "note" %}}
All these mutations require passing an `X-Dgraph-AccessToken` header, value for which can be obtained after logging in.
{{% /notice %}}

1) Reset the root password. The example below uses the dgraph endpoint `localhost:8080/admin`as a demo, make sure to choose the correct IP and port for your environment:
```graphql
mutation {
  updateUser(input: {filter: {name: {eq: "groot"}}, set: {password: "newpassword"}}) {
    user {
      name
    }
  }
}
```
The default password is `password`. `groot` is part of a special group called `guardians`. Members of `guardians` group will have access to everything. You can add more users to this group if required.

2) Create a regular user

```graphql
mutation {
  addUser(input: [{name: "alice", password: "newpassword"}]) {
    user {
      name
    }
  }
}
```

Now you should see the following output

```json
{
  "data": {
    "addUser": {
      "user": [
        {
          "name": "alice"
        }
      ]
    }
  }
}
```

3) Create a group

```graphql
mutation {
  addGroup(input: [{name: "dev"}]) {
    group {
      name
      users {
        name
      }
    }
  }
}
```

Now you should see the following output

```json
{
  "data": {
    "addGroup": {
      "group": [
        {
          "name": "dev",
          "users": []
        }
      ]
    }
  }
}
```

4) Assign the user to the group
To assign the user `alice` to both the group `dev` and the group `sre`, the mutation should be

```graphql
mutation {
  updateUser(input: {filter: {name: {eq: "alice"}}, set: {groups: [{name: "dev"}, {name: "sre"}]}}) {
    user {
      name
      groups {
        name
    }
    }
  }
}
```

5) Assign predicate permissions to the group

```graphql
mutation {
  updateGroup(input: {filter: {name: {eq: "dev"}}, set: {rules: [{predicate: "friend", permission: 7}]}}) {
    group {
      name
      rules {
        permission
        predicate
      }
    }
  }
}
```
Here we assigned a permission rule for the friend predicate to the group. In case you have [reverse edges]({{< relref "query-language/index.md#reverse-edges" >}}), they have to be given the permission to the group as well
```graphql
mutation {
  updateGroup(input: {filter: {name: {eq: "dev"}}, set: {rules: [{predicate: "~friend", permission: 7}]}}) {
    group {
      name
      rules {
        permission
        predicate
      }
    }
  }
}
```
You can also resolve this by using the `dgraph acl` tool
```
dgraph acl -a <ALPHA_ADDRESS:PORT> -w <GROOT_USER> -x <GROOT_PASSWORD>  mod --group dev --pred ~friend --perm 7
```

The command above grants the `dev` group the `READ`+`WRITE`+`MODIFY` permission on the
`friend` predicate. Permissions are represented by a number following the UNIX file
permission convention. That is, 4 (binary 100) represents `READ`, 2 (binary 010)
represents `WRITE`, and 1 (binary 001) represents `MODIFY` (the permission to change a
predicate's schema). Similarly, permisson numbers can be bitwise OR-ed to represent
multiple permissions. For example, 7 (binary 111) represents all of `READ`, `WRITE` and
`MODIFY`. In order for the example in the next section to work, we also need to grant
full permissions on another predicate `name` to the group `dev`. If there are no rules for
a predicate, the default behavior is to block all (`READ`, `WRITE` and `MODIFY`) operations.

```graphql
mutation {
  updateGroup(input: {filter: {name: {eq: "dev"}}, set: {rules: [{predicate: "name", permission: 7}]}}) {
    group {
      name
      rules {
        permission
        predicate
      }
    }
  }
}
```

### Retrieve Users and Groups Information
{{% notice "note" %}}
All these queries require passing an `X-Dgraph-AccessToken` header, value for which can be obtained after logging in.
{{% /notice %}}
The following examples show how to retrieve information about users and groups.

#### Using a GraphQL tool

1) Check information about a user

```graphql
query {
  getUser(name: "alice") {
    name
    groups {
      name
    }
  }
}
```

and the output should show the groups that the user has been added to, e.g.

```json
{
  "data": {
    "getUser": {
      "name": "alice",
      "groups": [
        {
          "name": "dev"
        }
      ]
    }
  }
}
```

2) Check information about a group

```graphql
{
  getGroup(name: "dev") {
    name
    users {
      name
    }
    rules {
      permission
      predicate
    }
  }
}
```

and the output should include the users in the group, as well as the permissions, the
group's ACL rules, e.g.

```json
{
  "data": {
    "getGroup": {
      "name": "dev",
      "users": [
        {
          "name": "alice"
        }
      ],
      "rules": [
        {
          "permission": 7,
          "predicate": "friend"
        },
        {
          "permission": 7,
          "predicate": "name"
        }
      ]
    }
  }
}
```

3) Query for users

```graphql
query {
  queryUser(filter: {name: {eq: "alice"}}) {
    name
    groups {
      name
    }
  }
}
```

and the output should show the groups that the user has been added to, e.g.

```json
{
  "data": {
    "queryUser": [
      {
        "name": "alice",
        "groups": [
          {
            "name": "dev"
          }
        ]
      }
    ]
  }
}
```

4) Query for groups

```graphql
query {
  queryGroup(filter: {name: {eq: "dev"}}) {
    name
    users {
      name
    }
    rules {
      permission
      predicate
    }
  }
}
```

and the output should include the users in the group, as well as the permissions the
group's ACL rules, e.g.

```json
{
  "data": {
    "queryGroup": [
      {
        "name": "dev",
        "users": [
          {
            "name": "alice"
          }
        ],
        "rules": [
          {
            "permission": 7,
            "predicate": "friend"
          },
          {
            "permission": 7,
            "predicate": "name"
          }
        ]
      }
    ]
  }
}
```

5) Run ACL commands as another guardian (member of `guardians` group).

You can also run ACL commands with other users. Say we have a user `alice` which is member
of `guardians` group and its password is `simple_alice`.

### Access Data Using a Client

Now that the ACL data are set, to access the data protected by ACL rules, we need to
first log in through a user. This is tyically done via the client's `.login(USER_ID, USER_PASSWORD)` method.

A sample code using the dgo client can be found
[here](https://github.com/dgraph-io/dgraph/blob/master/tlstest/acl/acl_over_tls_test.go). An example using
dgraph4j can be found [here](https://github.com/dgraph-io/dgraph4j/blob/master/src/test/java/io/dgraph/AclTest.java).

### Access Data Using the GraphQL API

Dgraph's HTTP API also supports authenticated operations to access ACL-protected
data.

To login, send a POST request to `/admin` with the GraphQL mutation. For example, to log in as the root user groot:

```graphql
mutation {
  login(userId: "groot", password: "password") {
    response {
      accessJWT
      refreshJWT
    }
  }
}
```

Response:

```json
{
  "data": {
    "accessJWT": "<accessJWT>",
    "refreshJWT": "<refreshJWT>"
  }
}
```

The response includes the access and refresh JWTs which are used for the authentication itself and refreshing the authentication token, respectively. Save the JWTs from the response for later HTTP requests.

You can run authenticated requests by passing the accessJWT to a request via the `X-Dgraph-AccessToken` header. Add the header `X-Dgraph-AccessToken` with the `accessJWT` value which you got in the login response in the GraphQL tool which you're using to make the request. For example:

```graphql
mutation {
  addUser(input: [{name: "alice", password: "newpassword"}]) {
    user {
      name
    }
  }
}
```

The refresh token can be used in the `/admin` POST GraphQL mutation to receive new access and refresh JWTs, which is useful to renew the authenticated session once the ACL access TTL expires (controlled by Dgraph Alpha's flag `--acl_access_ttl` which is set to 6h0m0s by default).

```graphql
mutation {
  login(userId: "groot", password: "newpassword", refreshToken: "<refreshJWT>") {
    response {
      accessJWT
      refreshJWT
    }
  }
}
```

### Reset Groot Password

If you've forgotten the password to your groot user, then you may reset the groot password (or
the password for any user) by following these steps.

1. Stop Dgraph Alpha.
2. Turn off ACLs by removing the `--acl_hmac_secret` config flag in the Alpha config. This leaves
   the Alpha open with no ACL rules, so be sure to restrict access, including stopping request
   traffic to this Alpha.
3. Start Dgraph Alpha.
4. Connect to Dgraph Alpha using Ratel and run the following upsert mutation to update the groot password
   to `newpassword` (choose your own secure password):
   ```text
   upsert {
     query {
       groot as var(func: eq(dgraph.xid, "groot"))
     }
     mutation {
       set {
         uid(groot) <dgraph.password> "newpassword" .
       }
     }
   }
   ```
5. Restart Dgraph Alpha with ACLs turned on by setting the `--acl_hmac_secret` config flag.
6. Login as groot with your new password.

## Encryption at Rest

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

### Set up Encryption

To enable encryption, we need to pass a file that stores the data encryption key with the option
`--encryption_key_file`. The key size must be 16, 24, or 32 bytes long, and the key size determines
the corresponding block size for AES encryption ,i.e. AES-128, AES-192, and AES-256, respectively.

You can use the following command to create the encryption key file (set _count_ to the
desired key size):

```
dd if=/dev/random bs=1 count=32 of=enc_key_file
```

### Turn on Encryption

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

### Turn off Encryption

If you wish to turn off encryption from an existing Alpha, then you can export your data and import it
into a new Dgraph instance without encryption enabled.

### Change Encryption Key

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
