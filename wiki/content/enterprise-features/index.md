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

Binary backups are full backups of Dgraph that are backed up directly to cloud
storage such as Amazon S3 or any Minio storage backend. Backups can also be
saved to an on-premise network file system shared by all alpha instances. These
backups can be used to restore a new Dgraph cluster to the previous state from
the backup. Unlike [exports]({{< relref "deploy/index.md#export-database" >}}),
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

To create a backup, make an HTTP POST request to `/admin/backup` to a Dgraph
Alpha HTTP address and port (default, "localhost:8080"). Like with all `/admin`
endpoints, this is only accessible on the same machine as the Alpha unless
[whitelisted for admin operations]({{< relref "deploy/index.md#whitelist-admin-operations" >}}).

#### Backup to Amazon S3

```sh
$ curl -XPOST localhost:8080/admin/backup -d "destination=s3://s3.us-west-2.amazonaws.com/<bucketname>"
```

#### Backup to Minio

```sh
$ curl -XPOST localhost:8080/admin/backup -d "destination=minio://127.0.0.1:9000/<bucketname>"
```

#### Disabling HTTPS for S3 and Minio backups

By default, Dgraph assumes the destination bucket is using HTTPS. If that is not
the case, the backup will fail. To send a backup to a bucket using HTTP
(insecure), set the query parameter `secure=false` with the destination
endpoint:

```sh
$ curl -XPOST localhost:8080/admin/backup -d "destination=minio://127.0.0.1:9000/<bucketname>?secure=false"
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

```
# localhost:8080 is the default Alpha HTTP port
$ curl -XPOST localhost:8080/admin/backup -d "destination=/path/to/local/directory"
```

A local filesystem will work only if all the Alphas have access to it (e.g all
the Alphas are running on the same filesystems as a normal process, not a Docker
container). However, a NFS is recommended so that backups work seamlessly across
multiple machines and/or containers.

#### Forcing a Full Backup

By default, an incremental backup will be created if there's another full backup
in the specified location. To create a full backup, set the `force_full` parameter
to `true`. Each series of backups can be
identified by a unique ID and each backup in the series is assigned a
monotonically increasing number. The following section contains more details on
how to restore a backup series.

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
directory are copied over to the machines/containers running the alphas and
`dgraph alpha` is started to load the copied data. For example, in a database
cluster with two Alpha groups and one replica each, `p1` is moved to the
location of the first Alpha and `p2` is moved to the location of the second
Alpha.

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

Now that your cluster is running with the ACL feature turned on, let's set up the ACL
rules. A typical workflow is the following:

1. Reset the root password. The example below uses the dgraph endpoint `localhost:9080`
as a demo, make sure to choose the correct IP and port for your environment:
```bash
dgraph acl -a localhost:9080 mod -u groot --new_password
```
Now type in the password for the groot account, which is the superuser that has access to everything. The default password is `password`.
`groot` is part of a special group called `guardians`. Members of `guardians` group will have access to everything. We can add more users
to this group if required.
2. Create a regular user
```bash
dgraph acl -a localhost:9080 add -u alice
```
Now you should see the following output
```console
Current password for groot:
Running transaction with dgraph endpoint: localhost:9080
Login successful.
New password for alice:
Retype new password for alice:
Created new user with id alice
```
3. Create a group
```bash
dgraph acl -a localhost:9080 add -g dev
```
Again type in the groot password, and you should see the following output
```console
Current password for groot:
Running transaction with dgraph endpoint: localhost:9080
Login successful.
Created new group with id dev
```
4. Assign the user to the group
```bash
dgraph acl -a localhost:9080 mod -u alice -l dev
```
The command above will add `alice` to the `dev` group. A user can be assigned to multiple groups.
The multiple groups should be formated as a single string separated by `,`.
For example, to assign the user `alice` to both the group `dev` and the group `sre`, the command should be
```bash
dgraph acl -a localhost:9080 mod -u alice -l dev,sre
```
5. Assign predicate permissions to the group
```bash
dgraph acl mod -a localhost:9080 -g dev -p friend -m 7
```
The command above grants the `dev` group the `READ`+`WRITE`+`MODIFY` permission on the
`friend` predicate. Permissions are represented by a number following the UNIX file
permission convention. That is, 4 (binary 100) represents `READ`, 2 (binary 010)
represents `WRITE`, and 1 (binary 001) represents `MODIFY` (the permission to change a
predicate's schema). Similarly, permisson numbers can be bitwise OR-ed to represent
multiple permissions. For example, 7 (binary 111) represents all of `READ`, `WRITE` and
`MODIFY`. In order for the example in the next section to work, we also need to grant
full permissions on another predicate `name` to the group `dev`. If there are no rules for
a predicate, the default behavior is to block all (`READ`, `WRITE` and `MODIFY`) operation.
```bash
dgraph acl mod -a localhost:9080 -g dev -p name -m 7
```
6. Check information about a user
```bash
dgraph acl info -a localhost:9080 -u alice
```
and the output should show the groups that the user has been added to, e.g.
```bash
Running transaction with dgraph endpoint: localhost:9080
Login successful.
User  : alice
UID   : 0x3
Group : dev
Group : sre
```
7. Check information about a group
```bash
dgraph acl info -a localhost:9080 -g dev
```
and the output should include the users in the group, as well as the permissions the
group's ACL rules, e.g.
```bash
Current password for groot:
Running transaction with dgraph endpoint: localhost:9080
Login successful.
Group: dev
UID  : 0x4
ID   : dev
Users: alice
ACL  : {friend  7}
ACL  : {name  7}
```

8. Run ACL commands as another guardian (Member of `guardians` group)
We can also run ACL commands with other users. Say we have a user `alice` which is member
of `guardians` group and its password is `simple_alice`. We can run ACL commands as shown below.
```bash
dgraph acl info -a localhost:9180 -u groot -w alice -x simple_alice
```
### Access Data Using a Client

Now that the ACL data are set, to access the data protected by ACL rules, we need to
first log in through a user. A sample code using the dgo client can be found
[here](https://github.com/dgraph-io/dgraph/blob/master/tlstest/acl/acl_over_tls_test.go).

### Access Data Using Curl

Dgraph's HTTP API also supports authenticated operations to access ACL-protected
data.

To login, send a POST request to `/login` with the userid and password. For example, to log in as the root user groot:

```sh
$ curl -X POST localhost:8080/login -d '{
  "userid": "groot",
  "password": "password"
}'
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

You can run authenticated requests by passing the accessJWT to a request via the `X-Dgraph-AccessToken` header. For example:

```sh
$ curl -X POST -H 'Content-Type: application/graphql+-' -H 'X-Dgraph-AccessToken: <accessJWT>' localhost:8080/query -d '...'
$ curl -X POST -H 'Content-Type: application/json' -H 'X-Dgraph-AccessToken: <accessJWT>' localhost:8080/mutate -d '...'
$ curl -X POST -H 'X-Dgraph-AccessToken: <accessJWT>' localhost:8080/alter -d '...'
```

The refresh token can be used in the `/login` POST body to receive new access and refresh JWTs, which is useful to renew the authenticated session once the ACL access TTL expires (controlled by Dgraph Alpha's flag `--acl_access_ttl` which is set to 6h0m0s by default).

```sh
$ curl -X POST localhost:8080/login -d '{
  "refresh_token": "<refreshJWT>"
}'
```

## Encryption at Rest

Encryption at rest refers to the encryption of data that is stored physically in any digital
form. It ensures that sensitive data on disks is not readable by any user or application
without a valid key that is required for decryption. Dgraph provides encryption at rest as an
enterprise feature. If encryption is enabled, Dgraph uses AES (Advanced Encryption Standard)
algorithm to encrypt the data and secure it.

### Set up Encryption

To enable encryption, we need to pass a file that stores the data encryption key with the option
`--encryption_key_file`. The key size must be 16, 24, or 32 bytes long, and the key size determines
the corresponding block size for AES encryption ,i.e. AES-128, AES-192, and AES-256, respectively.

Here is an example encryption key file of size 16 bytes.

*enc_key_file*
```
123456789012345
```

### Turn on Encryption

Here is an example that starts one zero server and one alpha server with the encryption feature turned on:

```bash
dgraph zero --my=localhost:5080 --replicas 1 --idx 1
dgraph alpha --encryption_key_file "./enc_key_file" --my=localhost:7080 --lru_mb=1024 --zero=localhost:5080
```

### Bulk loader with Encryption

Even before Dgraph cluster starts, we can load data using bulk loader with encryption feature turned on.
Later we can point the generated `p` directory to a new alpha server.

Here's an example to run bulk loader with a key used to write encrypted data:

```bash
dgraph bulk --encryption_key_file "./enc_key_file" -f data.json.gz -s data.schema --map_shards=1 --reduce_shards=1 --http localhost:8000 --zero=localhost:5080
```
