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

### Configure backup

Backup is only enabled when a valid license file is supplied to a Zero server OR within the thirty
(30) day trial period, no exceptions.

#### Configure Amazon S3 credentials

To backup to Amazon S3, the Alpha must have the following AWS credentials set
via environment variables:

 Environment Variable                        | Description
 --------------------                        | -----------
 `AWS_ACCESS_KEY_ID` or `AWS_ACCESS_KEY`     | AWS access key with permissions to write to the destination bucket.
 `AWS_SECRET_ACCESS_KEY` or `AWS_SECRET_KEY` | AWS access key with permissions to write to the destination bucket.
 `AWS_SESSION_TOKEN`                         | AWS session token (if required).

#### Configure Minio credentials

To backup to Minio, the Alpha must have the following Minio credentials set via
environment variables:

 Environment Variable                        | Description
 --------------------                        | -----------
 `MINIO_ACCESS_KEY`                          | Minio access key with permissions to write to the destination bucket.
 `MINIO_SECRET_KEY`                          | Minio secret key with permissions to write to the destination bucket.

### Create a backup

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

#### Overriding credentials

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

#### Forcing a full backup

By default, an incremental backup will be created if there's another full backup
in the specified location. An user can force a full backup to be created, they
can set the `force_full` parameter to "true". Each series of backups can be
identified by a unique ID and each backup in the series is assigned a
monotonically increasing number. The following section contains more details on
how to restore a backup series.

### Restore from backup

The `dgraph restore` command restores the postings directory from a previously
created backup. Restore is intended to restore a backup to a new Dgraph cluster.
During a restore, a new Dgraph Zero may be running to fully restore the backup
state.

The `--location` (`-l`) flag specifies a source URI with Dgraph backup objects.
This URI supports all the schemes used for backup.

The `--posting` (`-p`) flag sets the posting list parent directory to store the
loaded backup files.

The `--zero` (`-z`) optional flag specifies a Dgraph Zero address to update the
start timestamp using the restored version. Otherwise, the timestamp must be
manually updated through Zero's HTTP 'assign' endpoint.

The `--backup_id` optional flag specifies the ID of the backup series to
restore. A backup series consists of a full backup and all the incremental
backups built on top of it. Each time a new full backup is created, a new backup
series with a different ID is started. The backup series ID is stored in each
`manifest.json` file stored in every backup folder.

The restore feature will create a cluster with as many groups as the original
cluster had at the time of the last backup. Restoring create a posting directory
`p<N>` corresponding to the backup group ID. For example, a backup for Alpha
group 2 would have the name ".../r32-g**2**.backup" and would be loaded to
posting directory "p**2**".

#### Restore from Amazon S3
```sh
$ dgraph restore -p /var/db/dgraph -l s3://s3.us-west-2.amazonaws.com/<bucketname>
```

#### Restore from Minio
```sh
$ dgraph restore -p /var/db/dgraph -l minio://127.0.0.1:9000/<bucketname>
```

#### Restore from local directory or NFS
```sh
$ dgraph restore -p /var/db/dgraph -l /var/backups/dgraph
```

#### Restore and update timestamp

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

1. Since ACL is an enterprise feature, make sure your use case is covered under a contract with Dgraph Labs Inc.
You can contact us by sending an email to [contact@dgraph.io](mailto:contact@dgraph.io) or post your request at [our discuss
forum](https://discuss.dgraph.io) to get an enterprise license.

2. Create a plain text file, and store a randomly generated secret key in it. The secret key is used
by Alpha servers to sign JSON Web Tokens (JWT). As you’ve probably guessed, it’s critical to keep
the secret key as a secret. Another requirement for the secret key is that it must have at least 256-bits, i.e. 32 ASCII characters, as we are using HMAC-SHA256 as the signing algorithm.

3. Start all the alpha servers in your cluster with the option `--acl_secret_file`, and make sure
they are all using the same secret key file created in Step 2.

Here is an example that starts one zero server and one alpha server with the ACL feature turned on:
```
dgraph zero --my=localhost:5080 --replicas 1 --idx 1 --bindall --expose_trace --profile_mode block --block_rate 10 --logtostderr -v=2
dgraph alpha --my=localhost:7080 --lru_mb=1024 --zero=localhost:5080 --logtostderr -v=3 --acl_secret_file ./hmac-secret
```

If you are using docker-compose, a sample cluster can be set up by:

1. `cd $GOPATH/src/github.com/dgraph-io/dgraph/compose/`

2. `make`

3. `./compose -e --acl_secret <path to your hmac secret file>`, after which a `docker-compose.yml` file will be generated.

4. `docker-compose up` to start the cluster using the `docker-compose.yml` generated above.

### Set up ACL rules

Now that your cluster is running with the ACL feature turned on, let's set up the ACL rules. A typical workflow is the following:

1. Reset the root password. The example below uses the dgraph endpoint `localhost:9180` as a demo, make sure to choose the correct one for your environment:
```bash
dgraph acl -a localhost:9180 mod -u groot --new_password
```
Now type in the password for the groot account, which is the superuser that has access to everything. The default password is `password`.

2. Create a regular user
```bash
dgraph acl -a localhost:9180 add -u alice
```
Now you should see the following output
```bash
Current password for groot:
Running transaction with dgraph endpoint: localhost:9180
Login successful.
New password for alice:
Retype new password for alice:
Created new user with id alice
```

3. Create a group
```bash
dgraph acl -a localhost:9180 add -g dev
```
Again type in the groot password, and you should see the following output
```bash
Current password for groot:
Running transaction with dgraph endpoint: localhost:9180
Login successful.
Created new group with id dev
```
4. Assign the user to the group
```bash
dgraph acl -a localhost:9180 mod -u alice -l dev
```
The command above will add `alice` to the `dev` group. A user can be assigned to multiple groups.
The multiple groups should be formated as a single string separated by `,`.
For example, to assign the user `alice` to both the group `dev` and the group `sre`, the command should be
```bash
dgraph acl -a localhost:9180 mod -u alice -l dev,sre
```
5. Assign predicate permissions to the group
```bash
dgraph acl mod -a localhost:9180 -g dev -p friend -m 7
```
The command above grants the `dev` group the `READ`+`WRITE`+`MODIFY` permission on the `friend` predicate. Permissions are represented by a number following the UNIX file permission convention.
That is, 4 (binary 100) represents `READ`, 2 (binary 010) represents `WRITE`, and 1 (binary 001) represents `MODIFY` (the permission to change a predicate's schema). Similarly, permisson numbers can be bitwise OR-ed to represent multiple permissions. For example, 7 (binary 111) represents all of `READ`, `WRITE` and `MODIFY`.
In order for the example in the next section to work, we also need to grant full permissions on another predicate `name` to the group `dev`
```bash
dgraph acl mod -a localhost:9180 -g dev -p name -m 7
```

6. Check information about a user
```bash
dgraph acl info -a localhost:9180 -u alice
```
and the output should show the groups that the user has been added to, e.g.
```bash
Running transaction with dgraph endpoint: localhost:9180
Login successful.
User  : alice
UID   : 0x3
Group : dev
Group : sre
```

7. Check information about a group
```bash
dgraph acl info -a localhost:9180 -g dev
```
and the output should include the users in the group, as well as the permissions the group's ACL rules, e.g.
```bash
Current password for groot:
Running transaction with dgraph endpoint: localhost:9180
Login successful.
Group: dev
UID  : 0x4
ID   : dev
Users: alice
ACL  : {friend  7}
ACL  : {name  7}
```

### Access data using a client

Now that the ACL data are set, to access the data protected by ACL rules, we need to first log in through a user.
A sample code using the dgo client can be found [here](https://github.com/dgraph-io/dgraph/blob/master/tlstest/acl/acl_over_tls_test.go)
