# Binary Backups to Network File System

When using a file system for binary backups, NFS is recommended.  NFS will allow *"backups work seamlessly across multiple machines and/or containers"*.

* [Overview of NFS Servers](#overview-of-nfs-servers)
* [Provision NFS Server Instructions](#provision-nfs-server-instructions)
  * [Using Remote Cloud Solutions](#using-remote-cloud-solutions)
  * [Using the Rook Solution](#using-the-rook-solution)
  * [Using a Local Vagrant Solution](#using-a-local-vagrant-solution)
    * [Vagrant Server](#vagrant-server)
    * [Vagrant Client (Optional)](#vagrant-client-optional)
    * [Vagrant Cleanup](#vagrant-cleanup)
* [Testing NFS with Docker Compose](#testing-nfs-with-docker-compose)
  * [Setup Env Vars for Docker Compose](#setup-env-vars-for-docker-compose)
  * [Start Docker Compose with NFS Volume](#start-docker-compose-with-nfs-volume)
  * [Docker Cleanup](#docker-cleanup)
* [Testing NFS with Kubernetes](#testing-nfs-with-kubernetes)
  * [Setup Env Vars for Kubernetes](#setup-env-vars-for-kubernetes)
  * [Deploy Using Helmfile](#deploy-using-helmfile)
  * [Cleanup Using Helmfile](#cleanup-using-helmfile)
  * [Minikube Notes](#minikube-notes)
    * [Minikube with Virtualbox](#minikube-with-virtualbox)
    * [Minikube with KVM](#minikube-with-kvm)
    * [Verify NFS between Minikube and Vagrant](#verify-nfs-between-minikube-and-vagrant)
* [Accessing Dgraph Services](#accessing-dgraph-services)
* [Trigger a Backup](#trigger-a-backup)

## Overview of NFS Servers

You can use external NFS outside of the [Docker](https://www.docker.com/) or [Kubernetes](https://kubernetes.io/) cluster, or deploy a container offering NFS services.  

For production environments, using an NFS server external to the cluster can increase availability in an event where [Kubernetes](https://kubernetes.io/) services get interrupted.  In more advanced scenarios, deploying a container offering NFS services where the storage is backed by high-speed storage such as [Ceph](https://ceph.io/) is beneficial for large datasets.  In this latter scenario, secondary storage such as an object store by the cloud provider could be used for greater availability in event of where Kubernetes services or the [Kubernetes](https://kubernetes.io/) cluster itself has a failure event.

This guide provides tips on how to back up Dgraph using NFS. For this scope, automation here covers the following:

* External NFS
  * Cloud Providers
      * AWS [EFS](https://aws.amazon.com/efs/) ([Elastic File System](https://aws.amazon.com/efs/))
      * [Google Cloud Filestore](https://cloud.google.com/filestore)
  * Local NFS Server
      * [Vagrant](https://www.vagrantup.com/) managed virtual server that implements Linux kernel-based NFS Server
* Internal NFS (deployed as a container)
  * [Rook](https://rook.io/) NFS operator to deploy a container offering NFS Server with [Genesha NFS Server](https://github.com/nfs-ganesha/nfs-ganesha/wiki)

## Provision NFS Server Instructions

### Using Remote Cloud Solutions

You can provision external NFS to use with your Dgraph cluster running on Kubernetes using these scripts.  Unlike object storage, such as S3 or GCS, this storage will not be accessible from the public Internet and so can only be accessed from within a private subnet.

* Shell Scripts
  * [Google Cloud Filestore](gcfs-cli/README.md) - provision FileStore using `gcloud`
* Terraform
  * [Google Cloud Filestore](gcfs-terraform/README.md) - use Filestore as NFS share on GKE.
  * [Amazon Elastic File System](efs-terraform/README.md) - use EFS as NFS share on EKS.

### Using the Rook Solution

You can use an internal NFS server running on Kubernetes with [Rook](https://rook.io/) NFS Operator.  To enable this, run the following before running the [Kubernetes Environment](#testing-nfs-with-kubernetes).  Both of these steps are required for this feature:

```bash
## Download Rook NFS Operator Manifests
charts/rook/fetch-operator.sh
## Setup Environment for using Rook NFS Server
cp charts/rook/env.sh env.sh
```

### Using a Local Vagrant Solution

The steps to configure NFS for your local operating system or distro can vary greatly<sup>†</sup>, so a [Vagrant](https://www.vagrantup.com/) example is provided.  This should work [Virtualbox](https://www.virtualbox.org/) provider on Windows, Mac, and Linux, as [Virtualbox](https://www.virtualbox.org/) creates routable IP addresses available to the host.  Therefore, this NFS server can be accessed from either [Docker](https://docs.docker.com/engine/) or [Minikube](https://github.com/kubernetes/minikube) environments.

† <sup><sub>Linux and macOS have native NFS implementations with macOS NFS configuration varying between macOS versions.  Windows Server has different [NFS Server implementations](https://docs.microsoft.com/en-us/windows-server/storage/nfs/nfs-overview) between Windows Server versions.  For Windows 10, there are open source options such as [Cygwin](https://www.cygwin.com/) or you can use Linux through [WSL](https://docs.microsoft.com/en-us/windows/wsl/install-win10)</sub></sup>

#### Vagrant Server

You can bring up the NFS server with:

```bash
vagrant up
```

This will configure `env.sh` to point to NFS server on the guest system.

#### Vagrant Client (Optional)

Optionally, if you would like to use Dgraph in a virtual machine, you can bring up the client:

```bash
## Launch Dgraph VM
vagrant up nfs-client
## Log into nfs client system
vagrant ssh
## Change directory to configuration
cd /vagrant
```

After this, you can follow [Docker Compose Usage](#docker-compose-usage) to access NFS.

#### Vagrant Cleanup

```bash
vagrant destroy
```

## Testing NFS with Docker Compose

### Setup Env Vars for Docker Compose

If you used automation from [Vagrant Solution](#using-local-vagrant-solution), you can skip this step.  

Otherwise, you will need to create a file named `env.sh` and configure the IP address (or DNS name) and exported NFS shared file path:

```bash
export NFS_PATH="<exported-nfs-share>"
export NFS_SERVER="<server-ip-address>"
```

### Start Docker Compose with NFS Volume

```bash
## Source required environments variables
source env.sh
## Start Docker Compose
docker-compose up --detach
```

### Docker Cleanup

When finished, you can remove containers and volume resource with:

```bash
docker-compose stop && docker-compose rm
docker volume ls | grep -q nfs_mount || docker volume rm nfs_nfsmount > /dev/null
```

## Testing NFS with Kubernetes

### Setup Env Vars for Kubernetes

If you used automation from local [Vagrant Solution](#using-local-vagrant-solution), [Rook Solution](#using-rook-solution) cloud solution with [EFS](./efs-terraform/README.md) or [Google Cloud Filestore](./gcfs-terraform/README.md), you can skip this step.  

Otherwise, you will need to create a file named `env.sh` and configure the IP address (or DNS name) and exported NFS shared file path:

```bash
export NFS_PATH="<exported-nfs-share>"
export NFS_SERVER="<server-ip-address>"
```

#### Deploy Using Helmfile

If you have [helmfile](https://github.com/roboll/helmfile#installation) and [helm-diff](https://github.com/databus23/helm-diff) installed, you can deploy Dgraph with NFS support for backups with this:

```bash
## Source required environments variables
source env.sh
## Deploy Dgraph (and optional Rook if Rook was enabled)
helmfile apply
```

#### Cleanup Using Helmfile

```bash
helmfile delete
```

### Minikube Notes

If you are using NFS with [Vagrant Solution](#using-local-vagrant-solution), you will need to park [minikube](https://github.com/kubernetes/minikube) on the same private network as Vagrant.

#### Minikube with Virtualbox

For [VirtualBox](https://www.virtualbox.org) environments, where both [Vagrant](https://www.vagrantup.com/) and [minikube](https://github.com/kubernetes/minikube) will use [Virtualbox](https://www.virtualbox.org), you can do the following:

```bash
## Vagrant should have been started with Virtualbox by default
export VAGRANT_DEFAULT_PROVIDER="virtualbox"
vagrant up

## Set Driver to Virtualbox (same as Vagrant provider)
minikube config set driver virtualbox
## Start a miniKube cluster
minikube start --host-only-cidr='192.168.123.1/24'
```

#### Minikube with KVM

When using vagrant with `libvirt` (see [vagrant-libvirt](https://github.com/vagrant-libvirt/vagrant-libvirt)), you can have [minikube](https://github.com/kubernetes/minikube) target the same network.

```bash
## Vagrant should have been started with KVM
export VAGRANT_DEFAULT_PROVIDER="libvirt"
vagrant up

## Check that Virtual Network Exists based on directory name, e.g. `nfs0`
virsh net-list

## Start minikube using the same virtual network as Vagrant, e.g. `nfs0`
minikube config set driver kvm2
minikube start --kvm-network nfs0
```

#### Verify NFS between Minikube and Vagrant

Next, verify that NFS share works between the Vagrant NFS server and client Dgraph Alpha pod running in [minikube](https://github.com/kubernetes/minikube).

Create a file from the client:

```bash
## Log into an Alpha pod
RELEASE="my-release"
kubectl -ti exec $RELEASE-dgraph-alpha-0 -- bash
## Create a file on NFS volume
date > /dgraph/backups/hello_world.txt
exit
```

Verify that file was copied to the server:

```bash
## Log into Vagrant NFS Server
vagrant ssh nfs-server
## Check Results
cat /srv/share/hello_world.txt
logout
```

## Accessing Dgraph Services

In the [Docker Compose Environment](#testing-nfs-with-docker-compose), Alpha will be accessible from http://localhost:8080.

In a [Kubernetes Environment](#testing-nfs-with-kubernetes), you will need to use port-forward to access these from `localhost`.

For Dgraph Alpha, you can use this to access it at http://localhost:8080:

```bash
RELEASE="my-release"
export ALPHA_POD_NAME=$(
 kubectl get pods \
  --namespace default \
  --selector "statefulset.kubernetes.io/pod-name=$RELEASE-dgraph-alpha-0,release=$RELEASE" \
  --output jsonpath="{.items[0].metadata.name}"
)

kubectl --namespace default port-forward $ALPHA_POD_NAME 8080:8080
```

## Trigger a Backup

In the [Kubernetes Environment](#testing-nfs-with-kubernetes), backups are scheduled automatically using the [Kubernetes CronJob](https://kubernetes.io/docs/concepts/workloads/controllers/cron-jobs/).  As long as the services are available locally (see [Accessing Dgraph Services](#accessing-dgraph-services)), we can trigger a backup using a `curl` command.

For the [Docker Compose Environment](#testing-nfs-with-docker-compose) you can do the following:

```bash
ALPHA_HOST="localhost"
BACKUP_PATH="/data/backups"

GRAPHQL="{\"query\": \"mutation { backup(input: {destination: \\\"$BACKUP_PATH\\\" forceFull: true}) { response { message code } } }\"}"
HEADER="Content-Type: application/json"

curl --silent --header "$HEADER" --request POST $ALPHA_HOST:8080/admin --data "$GRAPHQL"
```

For [Kubernetes Environment](#testing-nfs-with-kubernetes), after running port-forward, you can do the following:

```bash
ALPHA_HOST="localhost"
BACKUP_PATH="/dgraph/backups"

GRAPHQL="{\"query\": \"mutation { backup(input: {destination: \\\"$BACKUP_PATH\\\" forceFull: true}) { response { message code } } }\"}"
HEADER="Content-Type: application/json"

curl --silent --header "$HEADER" --request POST $ALPHA_HOST:8080/admin --data "$GRAPHQL"
```
