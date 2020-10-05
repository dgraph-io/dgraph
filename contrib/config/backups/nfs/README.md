# Binary Backups to Network File System

When using a file system for binary backups, NFS is recommended so that  work seamlessly across multiple machines and/or containers.

## Provisioning External NFS

For NFS, you can provision an NFS outside of either Docker or Kubernetes, and use this as a mountable volume for Dgrpah alpha containers or pods.  For this purpose, you the NFS from provided by the host operating system, use the Vagrant example, or in cloud environments use NFS provided from the cloud provider.

### Cloud Solutions

You can provision external NFS with the scripts for use with Dgraph cluster running on Kubernetes.  Running these will populate Helm chart configuration values. If you want use this with Docker, the Docker containers must be running within the cloud services, as unlike Object Storage, these services would not be available on the Public Internet.

* Shell Scripts
  * [Google Cloud Filestore](gcfs-cli/README.md) - provision FileStore using `gcloud`
* Terraform
  * [Google Cloud Filestore](gcfs-terraform/README.md) - use Filestore as NFS share on GKE.
  * [Amazon Elastic File System](efs-terraform/README.md) - use EFS as NFS share on EKS.

### Using Vagrant Example

As configuring NFS for your local operating system or distro can vary greatly, a [Vagrant](https://www.vagrantup.com/) example is provided.  This should work [Virtualbox](https://www.virtualbox.org/) provider on Windows, Mac, and Linux.  As [Virtualbox](https://www.virtualbox.org/) creates routable IP addresses, these should be accessible with from [Docker](https://docs.docker.com/engine/) or [MiniKube](https://github.com/kubernetes/minikube) environments.

#### Vagrant Server

You can bring up the NFS server with:

```bash
vagrant up
```

This will confiure `env.sh` to point to NFS server on the guest system.

#### Vagrant Client (Optional)

Optionally, if you would like to use Dgraph in a virtual machine, you can bring up the client:

```bash
## Launch Dgraph VM
vagrant up nfs-client
## Log into nfs client system
vagrant ssh
## Change direcotry to configuration
cd /vagrant
```

After this you can follow use [Docker Compose Usage](#docker-compose-usage) to access NFS.

#### Vagrant Cleanup

```bash
vagrant destroy
```

## Docker Compose Usage

### Setup Env Vars

Create a file named `env.sh` and configure the IP address (or DNS name) and exported NFS shared file path:

```bash
export NFS_PATH="<exported-nfs-share>"
export NFS_SERVER="<server-ip-address>"
```

### Start Docker Compose with NFS Volume

```bash
## Source required enviroments variables
. env.sh
## Start Docker Compose
docker-compose up --detach
```

### Access Ratel UI

* Ratel UI: http://localhost:8000
  * configuration for Alpha is http://localhost:8080

### Cleanup

When finished, you can remove containers and volume resource with:

```bash
docker-compose stop && docker-compose rm
docker volume ls | grep -q nfs_mount || docker volume rm nfs_nfsmount > /dev/null
```
