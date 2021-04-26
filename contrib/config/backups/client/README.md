# Backup Script

This backup script that supports many of the features in Dgraph, such as ACLs, Mutual TLS, REST or GraphQL API.  See `./dgraph-backup.sh --help` for all of the options.

## Requirements

* The scripts (`dgraph-backup.sh` and `compose-setup.sh`) require the following tools to run properly:
  * GNU `bash`
  * GNU `getopt`
  * GNU `grep`<sup>†</sup>
* These scripts were tested on the following environments:
  * macOS with Homebrew [gnu-getopt](https://formulae.brew.sh/formula/gnu-getopt) bottle and [grep](https://formulae.brew.sh/formula/grep) bottle,
  * [Ubuntu 20.04.1 (Focal Fossa)](https://releases.ubuntu.com/20.04/) (any modern Linux distro should work, such as the [dgraph/dgraph](https://hub.docker.com/r/dgraph/dgraph/) docker container), and
  * Windows with [MSYS2](https://www.msys2.org/).
* For the test demo environment, both [docker](https://docs.docker.com/engine/) and [docker-compose](https://docs.docker.com/compose/) are required.

† Some versions of macOS 10.x do not include have a compatible version of `grep`.  You need to have GNU grep in the path for this script to work.

## Important Notes

If you are using this script on a system other than alpha, we'll call this *backup workstation*, you should be aware of the following:

* **General**
  * the *backup workstation* will need to have access to the alpha server, e.g. `localhost:8080`
* **TLS**
  * when accessing alpha server secured by TLS, the *backup workstation* will need access to `ca.crt` created with `dgraph cert` in the path.
  * if Mutual TLS is used, the *backup workstation* will also need access to the client cert and key in the path.
* **`subpath` option**
  * when specifying sub-path that uses a datestamp, the *backup workstation* needs to have the same timestamp as the alpha server.
  * when backing up to a file path, such as NFS, the *backup workstation* will need access to the same file path at the same mount point, e.g. if `/dgraph/backups` is used on alpha, the same path `/dgraph/backups` has to be accessible on the *backup workstation*

## Demo (Test) with local file path

You can try out these features using [Docker Compose](https://docs.docker.com/compose/).  There's a `./compose-setup.sh` script that can configure the environment with the desired features.  As you need to have a common shared directory for file paths, you can use `alpha1` container to run the backup script and backup to the shared `/dgraph/backups` directory.

As an example of performing backups with a local mounted file path using ACLs, Encryption, and TLS, you can follow these steps:

1. Setup Environment and log into *backup workstation* (Alpha container):
   ```bash
   ## configure docker-compose environment
   ./compose-setup.sh --acl --enc --tls --make_tls_cert
   ## run demo
   docker-compose up --detach
   ## login into Alpha to use for backups
   docker exec --tty --interactive alpha1 bash
   ```
2. Trigger a full backup:
   ```bash
   ## trigger a backup on alpha1:8080
   ./dgraph-backup.sh \
     --alpha alpha1:8080 \
     --tls_cacert /dgraph/tls/ca.crt \
     --force_full \
     --location /dgraph/backups \
     --user groot \
     --password password
   ```
3. Verify Results
   ```bash
   ## check for backup files
   ls /dgraph/backups
   ```
4. Logout of the Alpha container
   ```bash
   exit
   ```
4. Cleanup when finished
   ```bash
   docker-compose stop && docker-compose rm
   ```

### Demo (Test) with S3 Buckets

This will have requirements for [Terraform](https://www.terraform.io/) and [AWS CLI](https://aws.amazon.com/cli/).  See [s3/README.md](../s3/README.md) for further information.  Because we do not need to share the same file path, we can use the host as the *backup workstation*:

1. Setup the S3 Bucket environment. Make sure to replace `<your-bucket-name-goes-here>` to an appropriate name.
   ```bash
   ## create the S3 Bucket + Credentials
   pushd ../s3/terraform
   cat <<-TFVARS > terraform.tfvars
   name   = "<your-bucket-name-goes-here>"
   region = "us-west-2"
   TFVARS
   terraform init && terraform apply
   cd ..
   ## start Dgraph cluster with S3 bucket support
   docker-compose up --detach
   ## set $BACKUP_PATH env var for triggering backups
   source env.sh
   popd
   ```
2. Trigger a backup
   ```bash
   ./dgraph-backup.sh \
     --alpha localhost:8080 \
     --force_full \
     --location $BACKUP_PATH
   ```
3. Verify backups were finished
   ```bash
   aws s3 ls s3://${BACKUP_PATH##*/}
   ```
4. Clean up when completed:
   ```bash
   ## remove the local Dgraph cluster
   pushd ../s3
   docker-compose stop && docker-compose rm

   ## empty the bucket of contents
   aws s3 rm s3://${BACKUP_PATH##*/}/ --recursive

   ## destroy the s3 bucket and IAM user
   cd terraform
   terraform destroy

   popd
   ```

### Demo (Test) with GCP via Minio Gateway

This will have requirements for [Terraform](https://www.terraform.io/) and [Google Cloud SDK](https://cloud.google.com/sdk).  See [gcp/README.md](../gcp/README.md) for further information.  Because we do not need to share the same file path, we can use the host as the *backup workstation*:

1. Setup the GCS Bucket environment. Make sure to replace `<your-bucket-name-goes-here>` and `<your-project-name-goes-here` to something appropriate.
   ```bash
   ## create GCS Bucket + Credentials
   pushd ../gcp/terraform
   cat <<-TFVARS > terraform.tfvars
   region     = "us-central1"
   project_id = "<your-project-name-goes-here>"
   name       = "<your-bucket-name-goes-here>"
   TFVARS
   terraform init && terraform apply
   cd ..
   ## set $PROJECT_ID and $BACKUP_BUCKET_NAME env vars
   source env.sh
   ## start the Dgraph cluster with MinIO Gateway support
   docker-compose up --detach

   popd
   ```
2. Trigger a full backup
   ```bash
   ./dgraph-backup.sh \
     --alpha localhost:8080 \
     --force_full \
     --location minio://gateway:9000/${BACKUP_BUCKET_NAME}
   ```
3. Verify backups were created
   ```bash
   gsutil ls gs://${BACKUP_BUCKET_NAME}/
   ```
4. Clean up when finished:
   ```bash
   ## remove the local Dgraph cluster
   pushd ../gcp
   docker-compose stop && docker-compose rm

   ## empty the bucket contents
   gsutil rm -r gs://${BACKUP_BUCKET_NAME}/*

   ## destroy the gcs bucket and google service account
   cd terraform
   terraform destroy

   popd
   ```

### Demo (Test) with Azure Blob via Minio Gateway

This will have requirements for [Terraform](https://www.terraform.io/) and [Azure CLI](https://docs.microsoft.com/en-us/cli/azure/install-azure-cli).  See [azure/README.md](../azure/README.md) for further information.  Because we do not need to share the same file path, we can use the host as the *backup workstation*:

1. Setup Azure Storage Blob environment. Replace `<your-storage-account-name-goes-here>`, `<your-container-name-goes-here>`, and `<your-resource-group-name-goes-here>` to something appropriate.
   ```bash
   ## create Resource Group, Storage Account, authorize Storage Account, Create Storage Container
   pushd ../azure/terraform
   export STORAGE_ACCOUNT_NAME="<your-storage-account-name-goes-here>"
   export CONTAINER_NAME="<your-container-name-goes-here>"
   cat <<-TFVARS > terraform.tfvars
   resource_group_name  = "<your-resource-group-name-goes-here>"
   storage_account_name = "$STORAGE_ACCOUNT_NAME"
   storage_container_name = "$CONTAINER_NAME"
   TFVARS
   terraform init && terraform apply
   cd ..
   ## start the Dgraph cluster with MinIO Gateway support
   docker-compose up --detach

   popd
   ```
2. Trigger a backup
   ```bash
   ./dgraph-backup.sh \
     --alpha localhost:8080 \
     --force_full \
     --location minio://gateway:9000/${CONTAINER_NAME}
   ```
3. Verify backups were created
   ```bash
   az storage blob list \
     --account-name ${STORAGE_ACCOUNT_NAME} \
     --container-name ${CONTAINER_NAME} \
     --output table
   ```
4. Clean up when finished:
   ```bash
   ## remove the local Dgraph cluster
   pushd ../azure
   docker-compose stop && docker-compose rm

   ## destroy the storage account, the storage container, and the resource group
   cd terraform
   terraform destroy

   popd
   ```
