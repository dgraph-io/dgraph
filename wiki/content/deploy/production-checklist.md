+++
date = "2017-03-20T22:25:17+11:00"
title = "Production Checklist"
weight = 20
[menu.main]
    parent = "deploy"
+++

This guide describes important setup recommendations for a production-ready Dgraph cluster.

{{% notice "note" %}}
In this guide, a node refers to a Dgraph instance unless specified otherwise.
{{% /notice %}}

A **Dgraph cluster** is comprised of multiple **Dgraph instances** (aka **nodes**) connected together to form a single distributed database. A Dgraph instance is either a **Dgraph Zero** or **Dgraph Alpha**, each of which serves a different role in the cluster.

There can be one or more **Dgraph clients** connected to Dgraph to perform database operations (queries, mutations, alter schema, etc.). These clients connect via gRPC or HTTP. Dgraph provides official clients for Go, Java, Python, and JavaScript, and C#. All of these are gRPC-based, and JavaScript supports both gRPC and HTTP for browser support. Community-developed Dgraph clients for other languages are also available. The full list of clients can be found in [Clients]({{< relref "clients/_index.md" >}}) page. You can also interact with Dgraph via curl over HTTP. Dgraph Ratel is a UI client used to visualize queries, run mutations, and manage the schema. Clients do not participate as a member of the database cluster, they simply connect to one or more Dgraph Alpha instances.

### Cluster Requirements

A minimum of one Dgraph Zero and one Dgraph Alpha is needed for a working cluster.

There can be multiple Dgraph Zeros and Dgraph Alphas running in a single cluster.


### Machine Requirements

To ensure predictable performance characteristics, Dgraph instances should NOT run on "burstable" or throttled machines that limit resources. That includes t2 class machines on AWS.

We recommend each Dgraph instance to be deployed to a single dedicated machine to ensure that Dgraph can take full advantage of machine resources. That is, for a 6-node Dgraph cluster with 3 Dgraph Zeros and 3 Dgraph Alphas, each process runs in its own machine (e.g., EC2 instance). In the event of a machine failure, only one instance is affected, instead of multiple if they were running on that same machine.

If you'd like to run Dgraph with fewer machines, then the recommended configuration is to run a single Dgraph Zero and a single Dgraph Alpha per machine. In a high availability setup, that allows the cluster to lose a single machine (simultaneously losing a Dgraph Zero and a Dgraph Alpha) with continued availability of the database.

Do not run multiple Dgraph Zeros or Dgraph Alpha processes on a single machine. This can affect performance due to shared resources and reduce high availability in the event of machine failures.

### Operating System

The recommended operating system is Linux for production workloads. Dgraph provides release binaries in Linux, macOS, and Windows. For test environments, you can choose to run Dgraph on any operating system. This deployment guide assumes you are setting up Dgraph on Linux systems (see [Operating System Tuning]({{< relref
   "#operating-system-tuning" >}})).

### CPU and Memory

At the bare minimum, we recommend machines with at least 2 CPUs and 2 GiB of memory for testing.

You'll want a sufficient CPU and memory according to your production workload. A common setup for Dgraph is 16 CPUs and 32 GiB of memory per machine. Dgraph is designed with concurrency in mind, so more cores means quicker processing and higher throughput of requests.

You may find you'll need more CPU cores and memory for your specific use case.

### Disk

Dgraph instances make heavy usage of disks, so storage with high IOPS is highly recommended to ensure reliable performance. Specifically, we recommend SSDs, not HDDs.

Instances such as c5d.4xlarge have locally-attached NVMe SSDs with high IOPS. You can also use EBS volumes with provisioned IOPS (io1). If you are not running performance-critical workloads, you can also choose to use cheaper gp2 EBS volumes.

Recommended disk sizes for Dgraph Zero and Dgraph Alpha:

* Dgraph Zero: 200 GB to 300 GB. Dgraph Zero stores cluster metadata information and maintains a write-ahead log for cluster operations.
* Dgraph Alpha: 250 GB to 750 GB. Dgraph Alpha stores database data, including the schema, indices, and the data values. It maintains a write-ahead log of changes to the database. Your cloud provider may provide better disk performance based on the volume size.

Additional recommendations:

* The recommended Linux filesystem is ext4.
* Avoid using shared storage such as NFS, CIFS, and CEPH storage.

### Firewall Rules

Dgraph instances communicate over several ports. Firewall rules should be configured appropriately for the ports documented in [Ports Usage]({{< relref "deploy/ports-usage.md" >}}).

Internal ports must be accessible by all Zero and Alpha peers for proper cluster-internal communication. Database clients must be able to connect to Dgraph Alpha external ports either directly or through a load balancer.

Dgraph Zeros can be set up in a private network where communication is only with Dgraph Alphas, database administrators, internal services (such as Prometheus or Jaeger), and possibly developers (see note below). Dgraph Zero's 6080 external port is only necessary for database administrators. For example, it can be used to inspect the cluster metadata (/state), allocate UIDs or set txn timestamps (/assign), move data shards (/moveTablet), or remove cluster nodes (/removeNode). The full docs about Zero's administrative tasks are in [More About Dgraph Zero]({{< relref "deploy/dgraph-zero.md" >}}).

{{% notice "note" %}}
Developers using Dgraph Live Loader or Dgraph Bulk Loader require access to both Dgraph Zero port 5080 and Dgraph Alpha port 9080. When using those tools, consider using them within your environment that has network access to both ports of the cluster.
{{% /notice %}}

### Operating System Tuning

The OS should be configured with the recommended settings to ensure that Dgraph runs properly.

#### File Descriptors Limit

Dgraph can use a large number of open file descriptors. Most operating systems set a default limit that is lower than what is required.

It is recommended to set the file descriptors limit to unlimited. If that is not possible, set it to at least a million (1,048,576) which is recommended to account for cluster growth over time.

### Deployment

A Dgraph instance is run as a single process from a single static binary. It does not require any additional dependencies or separate services in order to run (see the [Supplementary Services]({{< relref "#supplementary-services" >}}) section for third-party services that work alongside Dgraph). A Dgraph cluster is set up by running multiple Dgraph processes networked together.

### Terminology

An **N-node cluster** is a Dgraph cluster that contains N number of Dgraph instances. For example, a 6-node cluster means six Dgraph instances. The **replication setting** specifies the number of Dgraph Alpha replicas are assigned per group. The replication setting is a configuration flag (`--replicas`) on Dgraph Zero. A **Dgraph Alpha group** is a set of Dgraph Alphas that store replications of the data amongst each other. Every Dgraph Alpha group is assigned a set of distinct predicates to store and serve.

Examples of different cluster settings:
* No sharding
  * 2-node cluster: 1 Zero, 1 Alpha (one group).
  * HA equivalent: x3 = 6-node cluster.
* With 2-way sharding:
  * 3-node cluster: 1 Zero, 2 Alphas (two groups).
  * HA equivalent: x3 = 9-node cluster.

In the following examples we outline the two most common cluster configurations: a 2-node cluster and a 6-node cluster.

### Basic setup: 2-node cluster
We provide sample configs for both [Docker Compose](https://github.com/dgraph-io/dgraph/blob/master/contrib/config/docker/docker-compose.yml) and [Kubernetes](https://github.com/dgraph-io/dgraph/tree/master/contrib/config/kubernetes/dgraph-single) for a 2-node cluster. You can also run Dgraph directly on your host machines.

{{% load-img "/images/deploy-guide-1.png" "2-node cluster" %}}

Configuration can be set either as command-line flags, environment variables, or in a config file (see [Config]({{< relref "deploy/config.md" >}})).

Dgraph Zero:
* The `--my` flag should be set to the address:port (the internal-gRPC port) that will be accessible to the Dgraph Alpha (default: `localhost:5080`).
* The `--idx` flag should be set to a unique Raft ID within the Dgraph Zero group (default: `1`).
* The `--wal` flag should be set to the directory path to store write-ahead-log entries on disk (default: `zw`).
* The `--bindall` flag should be set to true for machine-to-machine communication (default: `true`).
* Recommended: For better issue diagnostics, set the log level verbosity to 2 with the option `--v=2`.

Dgraph Alpha:
* The `--my` flag should be set to the address:port (the internal-gRPC port) that will be accessible to the Dgraph Zero (default: `localhost:7080`).
* The `--zero` flag should be set to the corresponding Zero address set for Dgraph Zero's `--my` flag.
* The `--postings` flag should be set to the directory path for data storage (default: `p`).
* The `--wal` flag should be set to the directory path for write-ahead-log entries (default: `w`)
* The `--bindall` flag should be set to true for machine-to-machine communication (default: `true`).
* Recommended: For better issue diagnostics, set the log level verbosity to 2 `--v=2`.

### HA setup: 6-node cluster

We provide sample configs for both [Docker Compose](https://github.com/dgraph-io/dgraph/blob/master/contrib/config/docker/docker-compose-ha.yml) and [Kubernetes](https://github.com/dgraph-io/dgraph/tree/master/contrib/config/kubernetes/dgraph-ha) for a 6-node cluster with 3 Alpha replicas per group. You can also run Dgraph directly on your host machines.

A Dgraph cluster can be configured in a high-availability setup with Dgraph Zero and Dgraph Alpha each set up with peers. These peers are part of Raft consensus groups, which elect a single leader amongst themselves. The non-leader peers are called followers. In the event that the peers cannot communicate with the leader (e.g., a network partition or a machine shuts down), the group automatically elects a new leader to continue.

Configuration can be set either as command-line flags, environment variables, or in a config file (see [Config]({{< relref "deploy/config.md" >}})).

In this setup, we assume the following hostnames are set:

- `zero1`
- `zero2`
- `zero3`
- `alpha1`
- `alpha2`
- `alpha3`

We will configure the cluster with 3 Alpha replicas per group. The cluster group-membership topology will look like the following:

{{% load-img "/images/deploy-guide-2.png" "Dgraph cluster image" %}}

#### Set up Dgraph Zero group

In the Dgraph Zero group you must set unique Raft IDs (`--idx`) per Dgraph Zero. Dgraph will not auto-assign Raft IDs to Dgraph Zero instances.

The first Dgraph Zero that starts will initiate the database cluster. Any following Dgraph Zero instances must connect to the cluster via the `--peer` flag to join. If the `--peer` flag is omitted from the peers, then the Dgraph Zero will create its own independent Dgraph cluster.

**First Dgraph Zero** example: `dgraph zero --replicas=3 --idx=1 --my=zero1:5080`

The `--my` flag must be set to the address:port of this instance that peers will connect to. The `--idx` flag sets its Raft ID to `1`.

**Second Dgraph Zero** example: `dgraph zero --replicas=3 --idx=2 --my=zero2:5080 --peer=zero1:5080`

The `--my` flag must be set to the address:port of this instance that peers will connect to. The `--idx` flag sets its Raft ID to 2, and the `--peer` flag specifies a request to connect to the Dgraph cluster of zero1 instead of initializing a new one.

**Third Dgraph Zero** example: `dgraph zero --replicas=3 --idx=3 --my=zero3:5080 --peer=zero1:5080`:

The `--my` flag must be set to the address:port of this instance that peers will connect to. The `--idx` flag sets its Raft ID to 3, and the `--peer` flag specifies a request to connect to the Dgraph cluster of zero1 instead of initializing a new one.

Dgraph Zero configuration options:

* The `--my` flag should be set to the address:port (the internal-gRPC port) that will be accessible to Dgraph Alpha (default: `localhost:5080`).
* The `--idx` flag should be set to a unique Raft ID within the Dgraph Zero group (default: `1`).
* The `--wal` flag should be set to the directory path to store write-ahead-log entries on disk (default: `zw`).
* The `--bindall` flag should be set to true for machine-to-machine communication (default: `true`).
* Recommended: For more informative log info, set the log level verbosity to 2 with the option `--v=2`.

#### Set up Dgraph Alpha group

The number of replica members per Alpha group depends on the setting of Dgraph Zero's `--replicas` flag. Above, it is set to 3. So when Dgraph Alphas join the cluster, Dgraph Zero will assign it to an Alpha group to fill in its members up to the limit per group set by the `--replicas` flag.

First Alpha example: `dgraph alpha --my=alpha1:7080 --zero=zero1:7080`

Second Alpha example: `dgraph alpha --my=alpha2:7080 --zero=zero1:7080`

First Alpha example: `dgraph alpha --my=alpha3:7080 --zero=zero1:7080`

Dgraph Alpha configuration options:

* The `--my` flag should be set to the address:port (the internal-gRPC port) that will be accessible to the Dgraph Zero (default: `localhost:7080`).
* The `--zero` flag should be set to the corresponding Zero address set for Dgraph Zero's `--my`flag.
* The `--postings` flag should be set to the directory path for data storage (default: `p`).
* The `--wal` flag should be set to the directory path for write-ahead-log entries (default: `w`)
* The `--bindall` flag should be set to true for machine-to-machine communication (default: `true`).
* Recommended: For more informative log info, set the log level verbosity to 2 `--v=2`.

### Supplementary Services

These services are not required for a Dgraph cluster to function but are recommended for better insight when operating a Dgraph cluster.

- [Metrics and monitoring][] with Prometheus and Grafana.
- [Distributed tracing][] with Jaeger.

[Metrics and monitoring]: {{< relref "deploy/monitoring.md" >}}
[Distributed tracing]: {{< relref "deploy/tracing.md" >}}
