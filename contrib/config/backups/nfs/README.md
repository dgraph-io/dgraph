# Binary Backups to Network File System

When using a file system for binary backups, NFS is recommended so that *backups work seamlessly across multiple machines and/or containers*.

## Provisioning NFS Overview

You can use external NFS outside of the [Docker](https://www.docker.com/) or [Kubernetes](https://kubernetes.io/), or deploy a container offering NFS services.  For production environments, using an NFS server external to the cluster can increase availability in an event where [Kubernetes](https://kubernetes.io/) services get interrupted. In more advanced scenarios, deploying a container offering NFS services, where the storage is backed by high-speed storage such as [Ceph](https://ceph.io/) is beneficial for large datasets.  In this latter scenario, secondary storage such as an object store by the cloud provider could be used for greater availability in event of where Kubernetes services or the [Kubernetes](https://kubernetes.io/) cluster itself has a failure event.

This guide is not meant to be complete, but rather to get you started on your backup journey with Dgraph and NFS.  For this scope, automation here covers the following:

* External NFS
  * Cloud Providers
    * [GCFS](https://cloud.google.com/filestore) ([Google Cloud Filestore](https://cloud.google.com/filestore))
    * AWS [EFS](https://aws.amazon.com/efs/) ([Elastic File System](https://aws.amazon.com/efs/))
  * Local NFS Server
    * [Vagrant](https://www.vagrantup.com/) managed virtual server that implements Linux kernel-based NFS Server
* Internal NFS (deployed as a container) 
    * [Rook](https://rook.io/) NFS operator to deploy container offering NFS Server with [Genesha NFS Server](https://github.com/nfs-ganesha/nfs-ganesha/wiki)

## Instructions

### Using Remote Cloud Solutions

You can provision external NFS with the scripts for use with the Dgraph cluster running on Kubernetes.  Unlike object storage, such as S3 or GCS, this storage will not be accessible from the public Internet, and so can only be accessed from within a private subnet.

* Shell Scripts
  * [Google Cloud Filestore](gcfs-cli/README.md) - provision FileStore using `gcloud`
* Terraform
  * [Google Cloud Filestore](gcfs-terraform/README.md) - use Filestore as NFS share on GKE.
  * [Amazon Elastic File System](efs-terraform/README.md) - use EFS as NFS share on EKS.

### Using Local Vagrant Solution

As configuring NFS for your local operating system or distro can vary greatly, a [Vagrant](https://www.vagrantup.com/) example is provided.  This should work [Virtualbox](https://www.virtualbox.org/) provider on Windows, Mac, and Linux, as [Virtualbox](https://www.virtualbox.org/) creates routable IP addresses available to the host.  Therefore, the NFS server can be accessed from either [Docker](https://docs.docker.com/engine/) or [MiniKube](https://github.com/kubernetes/minikube) environments.

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

After this, you can follow [Docker Compose Usage](#docker-compose-usage) to access NFS.

#### Vagrant Cleanup

```bash
vagrant destroy
```

## Testing NFS with Docker Compose

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
