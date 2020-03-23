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
continue enjoying the enterprise features released in the proprietary code. The license can
be applied to the cluster by including it as the body of a POST request and calling
`/enterpriseLicense` HTTP endpoint on any Zero server.


{{% notice "note" %}}
At the conclusion of your 30-day trial period if a license has not been applied to the cluster,
access to the enterprise features will be suspended. The cluster will continue to operate without
enterprise features.
{{% /notice %}}


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

The `access_key`, `secret_key`, and `session_token` parameters can be used to
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

The `--zero` (`-z`) optional flag specifies a Dgraph Zero address to update the
start timestamp using the restored version. Otherwise, the timestamp must be
manually updated through Zero's HTTP 'assign' endpoint.

The `--backup_id` optional flag specifies the ID of the backup series to
restore. A backup series consists of a full backup and all the incremental
backups built on top of it. Each time a new full backup is created, a new backup
series with a different ID is started. The backup series ID is stored in each
`manifest.json` file stored in every backup folder.

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
1. Reset the root password. The example below uses the dgraph endpoint `localhost:8080/admin`
as a demo, make sure to choose the correct IP and port for your environment:
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
2. Create a regular user
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
3. Create a group
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
4. Assign the user to the group
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
5. Assign predicate permissions to the group
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

1. Check information about a user
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
2. Check information about a group
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
3. Query for users 
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
4. Query for groups
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
5. Run ACL commands as another guardian (member of `guardians` group). 
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
```
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

## Encryption at Rest

{{% notice "note" %}}
This feature was introduced in [v1.1.1](https://github.com/dgraph-io/dgraph/releases/tag/v1.1.1).
For migrating unencrypted data to a new Dgraph cluster with encryption enabled, you need to
[export the database](https://docs.dgraph.io/deploy/#exporting-database) and [fast data load](https://docs.dgraph.io/deploy/#fast-data-loading),
preferably using the [bulk loader](https://docs.dgraph.io/deploy/#bulk-loader).
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

You can use the following command to create the encryption key file (set _count_ equals to the
desired key size):

```
dd if=/dev/random bs=1 count=32 of=enc_key_file
```

### Turn on Encryption

Here is an example that starts one zero server and one alpha server with the encryption feature turned on:

```bash
dgraph zero --my=localhost:5080 --replicas 1 --idx 1
dgraph alpha --encryption_key_file "./enc_key_file" --my=localhost:7080 --lru_mb=1024 --zero=localhost:5080
```

If multiple alpha nodes are part of the cluster, you will need to pass the `--encryption_key_file` option to
each of the alphas.

### Bulk loader with Encryption

Even before Dgraph cluster starts, we can load data using bulk loader with encryption feature turned on.
Later we can point the generated `p` directory to a new alpha server.

Here's an example to run bulk loader with a key used to write encrypted data:

```bash
dgraph bulk --encryption_key_file "./enc_key_file" -f data.json.gz -s data.schema --map_shards=1 --reduce_shards=1 --http localhost:8000 --zero=localhost:5080
```
