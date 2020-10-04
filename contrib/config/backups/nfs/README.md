# Binary Backups to Network File System

When using a file system for binary backups, NFS is recommended so that  work seamlessly across multiple machines and/or containers.

## Provisioning NFS

For NFS, you can provision an NFS outside of either Docker or Kubernetes, and use this as a mountable volume for Dgrpah alpha containers or pods.  For this purpose, you the NFS from provided by the host operating system, use the Vagrant example, or in cloud environments use NFS provided from the cloud provider.

### Cloud Solutons

You can provision extenral NFS with the scripts for use with Dgraph cluster running on Kubernetes.  Running these will populate Helm chart configuration values. If you want use this with Docker, the Docker containers must be running within th ecloud services, as unlike Object Storage, these services would not be available on the Public internet.

* Shell Scripts
  * [Google Cloud Filestore](gcfs-cli/README.md) - provision FileStore using `gcloud`
* Terraform
  * [Google Cloud Filestore](gcfs-terraform/README.md) - use Filestore as NFS share on GKE.
  * [Amazon Elastic File System](efs-terraform/README.md) - use EFS as NFS share on EKS.
