+++
date = "2017-03-20T22:25:17+11:00"
title = "Binary Backups"
weight = 1
[menu.main]
    parent = "enterprise-features"
+++

{{% notice "note" %}}
This feature was introduced in [v1.1.0](https://github.com/dgraph-io/dgraph/releases/tag/v1.1.0).
{{% /notice %}}

Binary backups are full backups of Dgraph that are backed up directly to cloud
storage such as Amazon S3 or any Minio storage backend. Backups can also be
saved to an on-premise network file system shared by all Alpha servers. These
backups can be used to restore a new Dgraph cluster to the previous state from
the backup. Unlike [exports]({{< relref "deploy/dgraph-administration.md#exporting-database" >}}),
binary backups are Dgraph-specific and can be used to restore a cluster quickly.


## Configure Backup

Backup is only enabled when a valid license file is supplied to a Zero server OR within the thirty
(30) day trial period, no exceptions.


### Configure Amazon S3 Credentials

To backup to Amazon S3, the Alpha server must have the following AWS credentials set
via environment variables:

 Environment Variable                        | Description
 --------------------                        | -----------
 `AWS_ACCESS_KEY_ID` or `AWS_ACCESS_KEY`     | AWS access key with permissions to write to the destination bucket.
 `AWS_SECRET_ACCESS_KEY` or `AWS_SECRET_KEY` | AWS access key with permissions to write to the destination bucket.
 `AWS_SESSION_TOKEN`                         | AWS session token (if required).


Starting with [v20.07.0](https://github.com/dgraph-io/dgraph/releases/tag/v20.07.0) if the system has access to the S3 bucket, you no longer need to explicitly include these environment variables.  

In AWS, you can accomplish this by doing the following:
1. Create an [IAM Role](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_create.html) with an IAM Policy that grants access to the S3 bucket.
2. Depending on whether you want to grant access to an EC2 instance, or to a pod running on [EKS](https://aws.amazon.com/eks/), you can do one of these options:
   * [Instance Profile](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_use_switch-role-ec2_instance-profiles.html) can pass the IAM Role to an EC2 Instance
   * [IAM Roles for Amazon EC2](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/iam-roles-for-amazon-ec2.html) to attach the IAM Role to a running EC2 Instance
   * [IAM roles for service accounts](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html) to associate the IAM Role to a [Kubernetes Service Account](https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/).


### Configure Minio Credentials

To backup to Minio, the Alpha server must have the following Minio credentials set via
environment variables:

 Environment Variable                        | Description
 --------------------                        | -----------
 `MINIO_ACCESS_KEY`                          | Minio access key with permissions to write to the destination bucket.
 `MINIO_SECRET_KEY`                          | Minio secret key with permissions to write to the destination bucket.


## Create a Backup

To create a backup, make an HTTP POST request to `/admin` to a Dgraph
Alpha HTTP address and port (default, "localhost:8080"). Like with all `/admin`
endpoints, this is only accessible on the same machine as the Alpha server unless
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

### Backup using a MinIO Gateway

#### Azure Blob Storage

You can use [Azure Blob Storage](https://azure.microsoft.com/services/storage/blobs/) through the [MinIO Azure Gateway](https://docs.min.io/docs/minio-gateway-for-azure.html).  You need to configure a [storage account](https://docs.microsoft.com/azure/storage/common/storage-account-overview) and a[container](https://docs.microsoft.com/azure/storage/blobs/storage-blobs-introduction#containers) to organize the blobs.

For MinIO configuration, you will need to [retrieve storage accounts keys](https://docs.microsoft.com/azure/storage/common/storage-account-keys-manage). The [MinIO Azure Gateway](https://docs.min.io/docs/minio-gateway-for-azure.html) will use `MINIO_ACCESS_KEY` and `MINIO_SECRET_KEY` to correspond to Azure Storage Account `AccountName` and `AccountKey`.

Once you have the `AccountName` and `AccountKey`, you can access Azure Blob Storage locally using one of these methods:

*  Run [MinIO Azure Gateway](https://docs.min.io/docs/minio-gateway-for-azure.html) using Docker
   ```bash
   docker run --publish 9000:9000 --name gateway \
     --env "MINIO_ACCESS_KEY=<AccountName>" \
     --env "MINIO_SECRET_KEY=<AccountKey>" \
     minio/minio gateway azure
   ```
*  Run [MinIO Azure Gateway](https://docs.min.io/docs/minio-gateway-for-azure.html) using the MinIO Binary
   ```bash
   export MINIO_ACCESS_KEY="<AccountName>"
   export MINIO_SECRET_KEY="<AccountKey>"
   minio gateway azure
   ```

#### Google Cloud Storage

You can use [Google Cloud Storage](https://cloud.google.com/storage) through the [MinIO GCS Gateway](https://docs.min.io/docs/minio-gateway-for-gcs.html).  You will need to [create storage buckets](https://cloud.google.com/storage/docs/creating-buckets), create a Service Account key for GCS and get a credentials file.  See [Create a Service Account key](https://github.com/minio/minio/blob/master/docs/gateway/gcs.md#11-create-a-service-account-ey-for-gcs-and-get-the-credentials-file) for further information.

Once you have a `credentials.json`, you can access GCS locally using one of these methods:

*  Run [MinIO GCS Gateway](https://docs.min.io/docs/minio-gateway-for-gcs.html) using Docker
   ```bash
   docker run --publish 9000:9000 --name gateway \
     --volume /path/to/credentials.json:/credentials.json \
     --env "GOOGLE_APPLICATION_CREDENTIALS=/credentials.json" \
     --env "MINIO_ACCESS_KEY=minioaccountname" \
     --env "MINIO_SECRET_KEY=minioaccountkey" \
     minio/minio gateway gcs <project-id>
   ```
*  Run [MinIO GCS Gateway](https://docs.min.io/docs/minio-gateway-for-gcs.html) using the MinIO Binary
   ```bash
   export GOOGLE_APPLICATION_CREDENTIALS=/path/to/credentials.json
   export MINIO_ACCESS_KEY=minioaccesskey
   export MINIO_SECRET_KEY=miniosecretkey
   minio gateway gcs <project-id>
   ```
 
#### Test Using MinIO Browser

MinIO Gateway comes with an embedded web-based object browser.  After using one of the aforementioned methods to run the MinIO Gateway, you can test that MinIO Gateway is running, open a web browser, navigate to http://127.0.0.1:9000, and ensure that the object browser is displayed and can access the remote object storage.

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

The `anonymous` parameter can be set to "true" to allow backing up to S3 or
MinIO bucket that requires no credentials (i.e a public bucket).


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

A local filesystem will work only if all the Alpha servers have access to it (e.g all
the Alpha servers are running on the same filesystems as a normal process, not a Docker
container). However, an NFS is recommended so that backups work seamlessly across
multiple machines and/or containers.


### Forcing a Full Backup

By default, an incremental backup will be created if there's another full backup
in the specified location. To create a full backup, set the `forceFull` field
to `true` in the mutation. Each series of backups can be
identified by a unique ID and each backup in the series is assigned a monotonically increasing number. The following section contains more details on how to restore a backup series.

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

## Listing backups.

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

- The requests should go to a single Alpha server. The Alpha server that receives the request
is responsible for looking up the location and determining from which point the
backup should resume.

- Versions of Dgraph starting with v20.07.1, v20.03.5, and v1.2.7 have a way to
block multiple backup requests going to the same Alpha server. For previous versions,
keep this in mind and avoid sending multiple requests at once. This is for the
same reason as the point above.

- You can have multiple backup series in the same location although the feature
still works if you set up a unique location for each series.

## Encrypted Backups

Encrypted backups are a Enterprise feature that are available from v20.03.1 and v1.2.3 and allow you to encrypt your backups and restore them. This documentation describes how to implement encryption into your binary backups.
Starting with v20.07.0, we also added support for Encrypted Backups using encryption keys sitting on Vault.


## New flag “Encrypted” in manifest.json

A new flag “Encrypted” is added to the `manifest.json`. This flag indicates if the corresponding binary backup is encrypted or not. To be backward compatible, if this flag is absent, it is presumed that the corresponding backup is not encrypted.

For a series of full and incremental backups, per the current design, we don't allow the mixing of encrypted and unencrypted backups. As a result, all full and incremental backups in a series must either be encrypted fully or not at all. This flag helps with checking this restriction.


## AES And Chaining with Gzip

If encryption is turned on an Alpha server, then we use the configured encryption key. The key size (16, 24, 32 bytes) determines AES-128/192/256 cipher chosen. We use the AES CTR mode. Currently, the binary backup is already gzipped. With encryption, we will encrypt the gzipped data.

During **backup**: the 16 bytes IV is prepended to the Cipher-text data after encryption.


## Backup

Backup is an online tool, meaning it is available when Alpha server is running. For encrypted backups, the Alpha server must be configured with the “encryption_key_file”. Starting with v20.07.0, the Alpha server can alternatively be configured to interface with Vault server to obtain keys.

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
		Number of the backup within the backup series to be restored. Backups with a greater value
		will be ignored. If the value is zero or is missing, the entire series will be restored.
		"""
		backupNum: Int

		"""
		Path to the key file needed to decrypt the backup. This file should be accessible
		by all Alpha servers in the group. The backup will be written using the encryption key
		with which the cluster was started, which might be different than this key.
		"""
		encryptionKeyFile: String

		"""
		Vault server address where the key is stored. This server must be accessible
		by all Alpha servers in the group. Default "http://localhost:8200".
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
restore, a new Dgraph Zero server may be running to fully restore the backup state.

The `--location` (`-l`) flag specifies a source URI with Dgraph backup objects.
This URI supports all the schemes used for backup.

The `--postings` (`-p`) flag sets the directory to which the restored posting
directories will be saved. This directory will contain a posting directory for
each group in the restored backup.

The `--zero` (`-z`) flag specifies a Dgraph Zero server address to update the start
timestamp and UID lease using the restored version. If no Zero server address is
passed, the command will complain unless you set the value of the
`--force_zero` flag to false. If do not pass a zero value to this command,
the timestamp and UID lease must be manually updated through Zero server's HTTP
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
example, a backup for Alpha Server group 2 would have the name `.../r32-g2.backup`
and would be loaded to posting directory `p2`.

After running the restore command, the directories inside the `postings`
directory need to be manually copied over to the machines/containers running the
Alpha servers before running the `dgraph alpha` command. For example, in a database
cluster with two Alpha groups and one replica each, `p1` needs to be moved to
the location of the first Alpha server and `p2` needs to be moved to the location of
the second Alpha server.

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

Specify the Zero server address and port for the new cluster with `--zero`/`-z` to update the timestamp.
```sh
$ dgraph restore -p /var/db/dgraph -l /var/backups/dgraph -z localhost:5080
```
