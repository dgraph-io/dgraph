# Binary Backups

These will be a collection of scripts to assist backup process for Binary Backups (Enterprise feature).

* Client
    * [Client](client/README.md) - a client `dgraph-backup.sh` that can used to automate backups.
* Cloud Object Storage
    * [Azure Blob Storage](azure/README.md) - use `minio` destination scheme with MinIO Azure Gateway to backup to Azure Blob Storage.
    * [GCS (Google Cloud Storage)](gcp/README.md) - use `minio` destination scheme with MinIO GCS Gateway to a GCS bucket.
    * [AWS S3 (Simple Storage Service)](s3/README.md) - use `s3` destination scheme to backup to an S3 bucket.
* File Storage
    * [NFS (Network File System)](nfs/README.md) - use file destination to backup to remote file storage
