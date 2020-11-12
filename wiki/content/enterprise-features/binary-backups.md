+++
date = "2017-03-20T22:25:17+11:00"
title = "Binary Backups"
[menu.main]
    parent = "enterprise-features"
    weight = 1
+++

{{% notice "note" %}}
This feature was introduced in [v1.1.0](https://github.com/dgraph-io/dgraph/releases/tag/v1.1.0).
{{% /notice %}}

Binary backups are full backups of Dgraph that are backed up directly to cloud
storage such as Amazon S3 or any Minio storage backend. Backups can also be
saved to an on-premise network file system shared by all alpha instances. These
backups can be used to restore a new Dgraph cluster to the previous state from
the backup. Unlike [exports]({{< relref "deploy/dgraph-administration.md#exporting-database" >}}),
binary backups are Dgraph-specific and can be used to restore a cluster quickly.


## Configure Backup

Backup is only enabled when a valid license file is supplied to a Zero server OR within the thirty
(30) day trial period, no exceptions.


### Configure Amazon S3 Credentials

To backup to Amazon S3, the Alpha must have the following AWS credentials set
via environment variables:

 Environment Variable                        | Description
 --------------------                        | -----------
 `AWS_ACCESS_KEY_ID` or `AWS_ACCESS_KEY`     | AWS access key with permissions to write to the destination bucket.
 `AWS_SECRET_ACCESS_KEY` or `AWS_SECRET_KEY` | AWS access key with permissions to write to the destination bucket.
 `AWS_SESSION_TOKEN`                         | AWS session token (if required).


### Configure Minio Credentials

To backup to Minio, the Alpha must have the following Minio credentials set via
environment variables:

 Environment Variable                        | Description
 --------------------                        | -----------
 `MINIO_ACCESS_KEY`                          | Minio access key with permissions to write to the destination bucket.
 `MINIO_SECRET_KEY`                          | Minio secret key with permissions to write to the destination bucket.


## Create a Backup

To create a backup, make an HTTP POST request to `/admin` to a Dgraph
Alpha HTTP address and port (default, "localhost:8080"). Like with all `/admin`
endpoints, this is only accessible on the same machine as the Alpha unless
[whitelisted for admin operations]({{< relref "deploy/dgraph-administration.md#whitelisting-admin-operations" >}}).
You can look at `BackupInput` given below for all the possible options.
```graphql
input BackupInput {

		"""
		Destination for the backup: e.g. Minio or S3 bucket.
		"""
		destination: String!

		"""
		Access key credential for the destination.
		"""
		accessKey: String

		"""
		Secret key credential for the destination.
		"""		
		secretKey: String

		"""
		AWS session token, if required.
		"""	
		sessionToken: String

		"""
		Set to true to allow backing up to S3 or Minio bucket that requires no credentials.
		"""	
		anonymous: Boolean

		"""
		Force a full backup instead of an incremental backup.
		"""	
		forceFull: Boolean
	}
```

Execute the following mutation on /admin endpoint using any GraphQL compatible client like Insomnia, GraphQL Playground or GraphiQL.

### Backup to Amazon S3

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


### Backup to Minio

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



### Backup to Minio


### Backup to Google Cloud Storage via Minio Gateway

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

### Test Using MinIO Browser
MinIO Gateway comes with an embedded web-based object browser that outputs content to http://127.0.0.1:9000. To test that MinIO Gateway is running, open a web browser, navigate to http://127.0.0.1:9000, and ensure that the object browser is displayed.
![](https://github.com/minio/minio/blob/master/docs/screenshots/minio-browser-gateway.png?raw=true)


### Test Using MinIO Client

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


### Disabling HTTPS for S3 and Minio backups

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


### Overriding Credentials

The `accessKey`, `secretKey`, and `sessionToken` parameters can be used to
override the default credentials. Please note that unless HTTPS is used, the
credentials will be transmitted in plain text so use these parameters with
discretion. The environment variables should be used by default but these
options are there to allow for greater flexibility.

The `anonymous` parameter can be set to "true" to a allow backing up to S3 or
Minio bucket that requires no credentials (i.e a public bucket).


### Backup to NFS

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


### Forcing a Full Backup

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

## Listing backups

The GraphQL admin interface includes the `listBackups` endpoint that lists the
backups in the given location along with the information included in their
`manifests.json` files. An example of a request to list the backups in the
`/data/backup` location is included below:

```
query backup() {
	listBackups(input: {location: "/data/backup"}) {
		backupId
		backupNum
		encrypted
		groups {
			groupId
			predicates
		}
		path
		since
		type
	}
}
```

The listBackups input can contain the following fields. Only the `location`
field is required.

```
input ListBackupsInput {
	"""
	Destination for the backup: e.g. Minio or S3 bucket.
	"""
	location: String!

	"""
	Access key credential for the destination.
	"""
	accessKey: String

	"""
	Secret key credential for the destination.
	"""
	secretKey: String

	"""
	AWS session token, if required.
	"""
	sessionToken: String

	"""
	Whether the destination doesn't require credentials (e.g. S3 public bucket).
	"""
	anonymous: Boolean
}
```

The output is of the `Manifest` type, which contains the fields below. The
fields correspond to the fields inside the `manifest.json` files.

```
type Manifest {
	"""
	Unique ID for the backup series.
	"""
	backupId: String

	"""
	Number of this backup within the backup series. The full backup always has a value of one.
	"""
	backupNum: Int

	"""
	Whether this backup was encrypted.
	"""
	encrypted: Boolean

	"""
	List of groups and the predicates they store in this backup.
	"""
	groups: [BackupGroup]

	"""
	Path to the manifest file.
	"""
	path: String

	"""
	The timestamp at which this backup was taken. The next incremental backup will
	start from this timestamp.
	"""
	since: Int

	"""
	The type of backup, either full or incremental.
	"""
	type: String
}

type BackupGroup {
	"""
	The ID of the cluster group.
	"""
	groupId: Int

	"""
	List of predicates assigned to the group.
	"""
	predicates: [String]
}
```

### Automating Backups

You can use the provided endpoint to automate backups, however, there are a few
things to keep in mind.

- The requests should go to a single alpha. The alpha that receives the request
is responsible for looking up the location and determining from which point the
backup should resume.

- Versions of Dgraph starting with v20.07.1, v20.03.5, and v1.2.7 have a way to
block multiple backup requests going to the same alpha. For previous versions,
keep this in mind and avoid sending multiple requests at once. This is for the
same reason as the point above.

- You can have multiple backup series in the same location although the feature
still works if you set up a unique location for each series.

## Encrypted Backups

Encrypted backups are a Enterprise feature that are available from v20.03.1 and v1.2.3 and allow you to encrypt your backups and restore them. This documentation describes how to implement encryption into your binary backups.
Starting with v20.07.0, we also added support for Encrypted Backups using encryption keys sitting on Vault. 


## New flag “Encrypted” in manifest.json

A new flag “Encrypted” is added to the `manifest.json`. This flag indicates if the corresponding binary backup is encrypted or not. To be backward compatible, if this flag is absent, it is presumed that the corresponding backup is not encrypted.

For a series of full and incremental backups, per the current design, we don't allow mixing of encrypted and unencrypted backups. As a result, all full and incremental backups in a series must either be encrypted fully or not at all. This flag helps with checking this restriction.


## AES And Chaining with Gzip

If encryption is turned on an alpha, then we use the configured encryption key. The key size (16, 24, 32 bytes) determines AES-128/192/256 cipher chosen. We use the AES CTR mode. Currently, the binary backup is already gzipped. With encryption, we will encrypt the gzipped data.

During **backup**: the 16 bytes IV is prepended to the Cipher-text data after encryption.


## Backup

Backup is an online tool, meaning it is available when alpha is running. For encrypted backups, the alpha must be configured with the “encryption_key_file”. Starting with v20.07.0, the alpha can alternatively be configured to interface with Vault server to obtain keys.

{{% notice "note" %}}
`encryption_key_file` or `vault_*` options was used for encryption-at-rest and will now also be used for encrypted backups.
{{% /notice %}}

## Online restore

To restore from a backup to a live cluster, execute a mutation on the `/admin`
endpoint with the following format.

```graphql
mutation{
  restore(input:{
    location: "/path/to/backup/directory",
    backupId: "id_of_backup_to_restore"'
  }){
    message
    code 
    restoreId
  }
}
```

Online restores only require you to send this request. The UID and timestamp
leases are updated accordingly. The latest backup to be restored should contain
the same number of groups in its manifest.json file as the cluster to which it
is being restored.

Restore can be performed from Amazon S3 / Minio or from a local directory. Below
is the documentation for the fields inside `RestoreInput` that can be passed into
the mutation.

```graphql
input RestoreInput {

		"""
		Destination for the backup: e.g. Minio or S3 bucket.
		"""
		location: String!

		"""
		Backup ID of the backup series to restore. This ID is included in the manifest.json file.
		If missing, it defaults to the latest series.
		"""
		backupId: String

		"""
		Path to the key file needed to decrypt the backup. This file should be accessible
		by all alphas in the group. The backup will be written using the encryption key
		with which the cluster was started, which might be different than this key.
		"""
		encryptionKeyFile: String

		"""
		Vault server address where the key is stored. This server must be accessible
		by all alphas in the group. Default "http://localhost:8200".
		"""
		vaultAddr: String

		"""
		Path to the Vault RoleID file.
		"""
		vaultRoleIDFile: String

		"""
		Path to the Vault SecretID file.
		"""
		vaultSecretIDFile: String

		"""
		Vault kv store path where the key lives. Default "secret/data/dgraph".
		"""
		vaultPath: String

		"""
		Vault kv store field whose value is the key. Default "enc_key".
		"""
		vaultField: String

		"""
		Vault kv store field's format. Must be "base64" or "raw". Default "base64".
		"""
		vaultFormat: String

		"""
		Access key credential for the destination.
		"""
		accessKey: String

		"""
		Secret key credential for the destination.
		"""		
		secretKey: String

		"""
		AWS session token, if required.
		"""	
		sessionToken: String

		"""
		Set to true to allow backing up to S3 or Minio bucket that requires no credentials.
		"""	
		anonymous: Boolean
}
```

Restore requests will return immediately without waiting for the operation to
finish. The `restoreId` value included in the response can be used to query for
the state of the restore operation via the `restoreStatus` endpoint. The request
should be sent to the same alpha to which the original restore request was sent.
Below is an example of how to perform the query.

```
query status() {
	restoreStatus(restoreId: 8) {
		status
		errors
	}
}
```

## Offline restore using `dgraph restore`

{{% notice "note" %}}
`dgraph restore` is being deprecated, please use GraphQL API for Restoring from Backup.
{{% /notice %}}

The restore utility is a standalone tool today. A new flag `--encryption_key_file` is added to the restore utility so it can decrypt the backup. This file must contain the same key that was used for encryption during backup.
Alternatively, starting with v20.07.0, the `vault_*` options can be used to restore a backup. 

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

The `--vault_*` flags specifies the Vault server address, role id, secret id and 
field that contains the encryption key that was used to encrypt the backup.

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


### Restore from Amazon S3
```sh
$ dgraph restore -p /var/db/dgraph -l s3://s3.us-west-2.amazonaws.com/<bucketname>
```


### Restore from Minio
```sh
$ dgraph restore -p /var/db/dgraph -l minio://127.0.0.1:9000/<bucketname>
```


### Restore from Local Directory or NFS
```sh
$ dgraph restore -p /var/db/dgraph -l /var/backups/dgraph
```


### Restore and Update Timestamp

Specify the Zero address and port for the new cluster with `--zero`/`-z` to update the timestamp.
```sh
$ dgraph restore -p /var/db/dgraph -l /var/backups/dgraph -z localhost:5080
```
