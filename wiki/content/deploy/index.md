+++
date = "2017-03-20T22:25:17+11:00"
title = "Deploy"
+++

This article should be followed only after having gone through [Get Started]({{< relref "get-started/index.md" >}}). If you haven't, please go through that first.

## Installation
If you followed one of the [recommended installation]({{< relref "get-started/index.md#step-1-installation" >}}) methods, then things should already be set correctly for you.

### Manual download (Optional)

If you don't want to follow the automatic installation method, you could manually download the appropriate tar for your platform from **[Dgraph releases](https://github.com/dgraph-io/dgraph/releases)**. After downloading the tar for your platform from Github, extract the binaries to `/usr/local/bin` like so.

```sh
# For Linux
$ sudo tar -C /usr/local/bin -xzf dgraph-linux-amd64-VERSION.tar.gz

# For Mac
$ sudo tar -C /usr/local/bin -xzf dgraph-darwin-amd64-VERSION.tar.gz
```

## Endpoints

On its port, a running Dgraph instance exposes a number of service endpoints.

* `/` Browser UI and query visualization.
* `/query` receive queries and respond in JSON.
* `/share`
* `/health` HTTP status code 200 and "OK" message if worker is running, HTTP 503 otherwise.
<!-- * `/debug/store` backend storage stats.-->
* `/admin/shutdown` [shutdown]({{< relref "#shutdown" >}}) a node.
* `/admin/backup` take a running [backup]({{< relref "#backup">}}).


## Running Dgraph

{{% notice "tip" %}}  All Dgraph tools have `--help`.  To view all the flags, run `dgraph --help`, it's a great way to familiarize yourself with the tools.{{% /notice %}}

Whether running standalone or in a cluster, each Dgraph instance relies on the following (if multiple instances are running on the same machine, instances cannot share these).

* A `p` directory.  This is where Dgraph persists the graph data as posting lists. (option `--p`, default: directory `p` where the instance was started)
* A `w` directory.  This is where Dgraph stores its write ahead logs. (option `--w`, default: directory `w` where the instance was started)
* The `p` and `w` directories must be different.
* A port exposing the instance for query, client connections and other [endpoints]({{< relref "#endpoints">}}). (option `--port`, default: `8080` on `localhost`)
* A port on which to run a worker node, used for Dgraph's communication between nodes. (option `--workerport`, default: `12345`)
* An address and port at which the node advertises its worker.  (option `--my`, default: `localhost:workerport`)

{{% notice "note" %}}By default `8080` or the port specified by `--port` is bound to `localhost` (the loopback address only accessible from the same machine).  The `--bindall=true` option binds to `0.0.0.0` and thus allows external connections. {{% /notice %}}

### Config
The command-line flags can be stored in a YAML file and provided via the `--config` flag.  For example:

```sh
# Folder in which to store backups.
backup: backup

# Fraction of dirty posting lists to commit every few seconds.
gentlecommit: 0.33

# RAFT ID that this server will use to join RAFT groups.
idx: 1

# Groups to be served by this instance.
groups: "0,1"

# Port to run server on. (default 8080)
port: 8080

# Port used by worker for internal communication.
workerport: 12345

# If RAM usage exceeds this, we stop the world, and flush our buffers.
stw_ram_mb: 4096

# The ratio of queries to trace.
trace: 0.33

# Directory to store posting lists.
p: p

# Directory to store raft write-ahead logs.
w: w

# Debug mode for testing.
debugmode: false
```

### TLS configuration
Connections between client and server can be secured with TLS.
Both encrypted (password protected) and unencrypted private keys are supported.

{{% notice "tip" %}}If you're generating encrypted private keys with `openssl`, be sure to specify encryption algorithm explicitly (like `-aes256`). This will force `openssl` to include `DEK-Info` header in private key, which is required to decrypt the key by Dgraph. When default encryption is used, `openssl` doesn't write that header and key can't be decrypted.{{% /notice %}}

Following configuration options are available for the server:

```sh
# Use TLS connections with clients.
tls.on

# CA Certs file path.
tls.ca_certs string

# Include System CA into CA Certs.
tls.use_system_ca

# Certificate file path.
tls.cert string

# Certificate key file path.
tls.cert_key string

# Certificate key passphrase.
tls.cert_key_passphrase string

# Enable TLS client authentication
tls.client_auth string

# TLS max version. (default "TLS12")
tls.max_version string

# TLS min version. (default "TLS11")
tls.min_version string
```

Dgraph loader can be configured with following options:

```sh
# Use TLS connections.
tls.on

# CA Certs file path.
tls.ca_certs string

# Include System CA into CA Certs.
tls.use_system_ca

# Certificate file path.
tls.cert string

# Certificate key file path.
tls.cert_key string

# Certificate key passphrase.
tls.cert_key_passphrase string

# Server name.
tls.server_name string

# Skip certificate validation (insecure)
tls.insecure

# TLS max version. (default "TLS12")
tls.max_version string

# TLS min version. (default "TLS11")
tls.min_version string
```

### Single Instance
A single instance can be run with default options, as in:

```sh
mkdir ~/dgraph # The folder where dgraph binary will create the directories it requires.
cd ~/dgraph
dgraph
```

Or by specifying `p` and `w` directories, ports, etc.  If `dgraphloader` is used, it must connect on the port exposing Dgraph services, as must the go client.  

### Multiple instances

Dgraph is a truly distributed graph database - not a master-slave replication of one datastore.  Dgraph shards by predicate, not by node.  Running in a cluster shards and replicates predicates across the cluster, queries can be run on any node and joins are handled over the distributed data.

As well as the requirements for [each instance]({{< relref "#running-dgraph">}}), to run Dgraph effectively in a cluster, it's important to understand how sharding and replication work.

* Dgraph stores data per predicate (not per node), thus the unit of sharding and replication is predicates.
* To shard the graph, predicates are assigned to groups and each node in the cluster serves a number of groups.
* Each node in a cluster stores only the predicates for the groups it is assigned to.
* If multiple cluster nodes server the same group, the data for that group is replicated

For example, if predicates `P1`, `P2` and `P3` are assigned to group 1, predicates `P4` and `P5` to group 2, and predicates `P6`, `P7` and `P8` to group 3.  If cluster node `N1` serves group 1, it stores data for only predicates `P1`, `P2` and `P3`.  While if node `N2` serves groups 1 and 3, it stores data for predicates `P1-P3` and `P6-P8`, replicating the `P1-P3` data.  A node `N3` could then, for example, serve groups 2 and 3.  A query is resolved locally for predicates the node stores and via distributed joins for predicates stored on other nodes.

Note also:

* Group 0 stores information about the cluster.
* If sharding results in `N` groups, then for every group `0,...,N` there must be at least one node serving the group.  If there are no nodes serving a particular group, then the cluster won't know where to store the data for predicates in that group.
* A Dgraph cluster can detect new machines allocated to the cluster, establish connections, and transfer a subset of existing predicates to the new node based on the groups served by the new machine.
* Similarly, machines can be taken down and then brought back up to serve different groups and Dgraph will reorganize for the new structure.


{{% notice "warning" %}}Group id 0 is used to store membership information for the entire cluster. Dgraph doesn't take snapshots of this group.  It is an error to assign a predicate to group 0. {{% /notice %}}


#### Data sharding

Sharding is specified by supplying the `--group_conf` flag.

The groups config syntax is as follows:

```
<groupID>: comma separated list of predicate names or prefixes

# Last entry should be:
default: fp % N + k, where N = number of shards you want, and k = starting shard id.
```

The default groups config used by Dgraph, when nothing is provided is:

```
$ cat cmd/dgraph/groups.conf
// Default formula for getting group where fp is the fingerprint of a predicate.
default: fp % 1 + 1

# fp % 1 is always zero. Thus, all data is located on group id 1.
```

{{% notice "note" %}} Assignment of predicates to groups is done in order of group ID.  If a predicate matches multiple groups, the lowest matching group is picked.{{% /notice %}}


A valid groups.conf is:

```
// Matching is by prefix when * is used, and by equality otherwise

1: type.object.name
2: type.object.name*, film.performance.*

// Default formula for getting group where fp is the fingerprint of a predicate.
default: fp % 10 + 2
```

For this groups.conf:

* Predicate `type.object.name` is assigned to group 1.
* Any predicate with prefix `type.object.name` and `film.performance.` will be assigned to group 2.
* `type.object.name` belongs to group 1 and not 2 despite matching both, because 1 is lower than 2.
* The remaining predicates are assigned by the formula: `fingerprint(predicate) % 10 + 2`, and thus occupy groups `[2, 3, 4, 5, 6, 7, 8, 9, 10, 11]`.
* Group 2 will serve predicates matching the specified prefixes and those set by the default rule.

{{% notice "note" %}} Data for reverse edges are always stored with the corresponding forward edge.  It's an error to use a reverse edge in groups.conf. {{% /notice %}}

{{% notice "warning" %}}Once sharding spec is set, it **must not be changed** without bringing the cluster down. The same spec must be passed to all the nodes in the cluster.{{% /notice %}}



#### Running the Cluster

Each machine in the cluster must be started with a unique ID (option `--idx`) and a comma-separated list of group IDs (option `--groups`).  Each machine must also satisfy the data directory and port requirements for a [single instance]({{< relref "#running-dgraph">}}).


To run a cluster, begin by bringing up a single server that serves at least group 0.  

```
$ dgraph --group_conf groups.conf --groups "0,1" --idx 1 --my "ip-address-others-should-access-me-at" --bindall=true

# This instance with ID 1 will serve groups 0 and 1, using the default 8080 port for clients and 12345 for peers.
```

{{% notice "note" %}} The `--bindall=true` option is required when running on multiple machines, otherwise the node's port and workerport will be bound to localhost and not be accessible over a network. {{% /notice %}}

New nodes are added to a cluster by specifying any known healthy node on startup (option `--peer`).  The address given at `--peer` must be the `workerport`.


```
# Server handling only group 2.
$ dgraph --group_conf groups.conf --groups "2" --idx 3 --peer "<ip address>:<workerport>" --my "ip-address-others-should-access-me-at" --bindall=true

# Server handling groups 0, 1 and 2.
$ dgraph --group_conf groups.conf --groups "0,1,2" --idx 4 --peer "<ip address>:<workerport>" --my "ip-address-others-should-access-me-at" --bindall=true
```

The new servers will automatically detect each other by communicating with the provided peer and establish connections to each other.

{{% notice "note" %}}To have RAFT consensus work correctly, each group must be served by an odd number of Dgraph instances.{{% /notice %}}

It can be worth building redundancy and extensibility into a cluster configuration so that a cluster can be extended online without needing to be restarted.  For example, by anticipating potential future shards and specifying a groups.conf file with more groups than initially needed - the first few instances might then serve many groups but it's easy to add more nodes as need arrises and even restart the initial nodes serving fewer groups once the cluster has enough redundancy.  If not enough groups are specified at the start, reconfiguration of the groups must be done offline.

Query patterns might also influence sharding.  There is no value in co-locating predicates that are never used in joins while distributing predicates that are often used together in joins.  Network communication is slower than memory, so considering common query patterns can lead to fewer distributed joins and fast query times.

#### Cluster Checklist

In setting up a cluster be sure the check the following.

* Is each dgraph instance in the cluster [set up correctly]({{< relref "#running-dgraph">}})?
* Will each instance be accessible to all peers on `workerport`?
* Is `groups.conf` configured to shard the predicates to groups correctly?
* Does each node have a unique ID on startup?
* Has `--bindall=true` been set for networked communication?
* Is a node serving group 0 being brought up first?
* Is every group going to be served by at least one node?


<!---
### In EC2

To make provisioning an EC2 Dgraph cluster quick, the following script installs and starts a Dgraph node, opens ports on the host machine and enters Dgraph into systemd so that the node will restart if the machine is rebooted.

We recommend an Ubuntu 16.04 AMI on a machine with at least 16GB of memory.  

The i3 instances with SSD drives have the right hardware set up to run Dgraph (and its underlying key value store Badger) for performance environments.

After provisioning an Ubuntu machine the following scripts installs and brings up Dgraph.


p w ip worker groups.conf groups heathy-peer

## Docker
--->

## Bulk Data Loading
The `dgraphloader` binary is a small helper program which reads RDF NQuads from a gzipped file, batches them up, creates mutations (using the go client) and shoots off to Dgraph. It's not the only way to run mutations.  Mutations could also be run from the command line, e.g. with `curl`, from the UI, by sending to `/query` or by a program using a [Dgraph client]({{< relref "clients/index.md" >}}).

`dgraphloader` correctly handles splitting blank nodes across multiple batches and creating `_xid_` [edges for RDF URIs]({{< relref "query-language/index.md#external-ids" >}}) (option `-x`).

{{% notice "note" %}} `dgraphloader` only accepts gzipped, RDF NQuad/Triple data. Data in other formats must be converted [to this](https://www.w3.org/TR/n-quads/).{{% /notice %}}



```
$ dgraphloader --help # To see the available flags.

# Read RDFs from the passed file, and send them to Dgraph on localhost:8080.
$ dgraphloader -r <path-to-rdf-gzipped-file>

# Read RDFs and a schema file and send do Dgraph running at given address
$ dgraphloader -r <path-to-rdf-gzipped-file> -s <path-to-schema-file> -d <dgraph-server-address:port>

# For example to load goldendata with the corresponding schema and convert URI to _xid_.
$ dgraphloader -r github.com/dgraph-io/benchmarks/data/goldendata.rdf.gz -s github.com/dgraph-io/benchmarks/data/goldendata.schema -x
```

## Backup

A backup of all nodes is started by locally accessing the backup endpoint of any server in the cluster.

```sh
$ curl localhost:8080/admin/backup
```
{{% notice "warning" %}}This won't work if called from outside the server where dgraph is running.  Ensure that the port is set to the port given by `--port` on startup. {{% /notice %}}

This also works from a browser, provided the HTTP GET is being run from the same server where the Dgraph instance is running.

This triggers a backup of all the groups spread across the entire cluster. Each server writes output in gzipped rdf to the backup directory specified on startup by `--backup`. If any of the groups fail, the entire backup process is considered failed, and an error is returned.

{{% notice "note" %}}It is up to the user to retrieve the right backups files from the servers in the cluster. Dgraph does not copy files  to the server that initiated the backup.{{% /notice %}}

## Shutdown

A clean exit of a single dgraph node is initiated by running the following command on that node.

```sh
$ curl localhost:8080/admin/shutdown
```
{{% notice "warning" %}}This won't work if called from outside the server where dgraph is running.  Ensure that the port is set to the port given by `--port` on startup.{{% /notice %}}

This stops the server on which the command is executed and not the entire cluster.

## Delete database

Individual triples, patterns of triples and predicates can be deleted as described in the [query languge docs]({{< relref "query-language/index.md#delete" >}}).  

To drop all data and start from a clean database:

* [stop Dgraph]({{< relref "#shutdown" >}}) and wait for all writes to complete,
* delete (maybe take a backup first) the `p` and `w` directories, then
* restart Dgraph.

## Upgrade Dgraph

<!--{{% notice "tip" %}}If you are upgrading from v0.7.3 you can modify the [schema file]({{< relref "query-language/index.md#schema">}}) to use the new syntax and give it to the dgraphloader using the `-s` flag while reloading your data.{{% /notice %}}-->

Doing periodic backups is always a good idea. This is particularly useful if you wish to upgrade Dgraph or reconfigure the sharding of a cluster. The following are the right steps safely backup and restart.

- Run a [backup]({{< relref "#backup">}})
- Ensure it's successful
- Bring down the cluster
- Upgrade Dgraph binary / specify a new groups.conf
- Run Dgraph using new data directories.
- Reload the data via [bulk data loading]({{< relref "#bulk-data-loading" >}}).
- If all looks good, you can delete the old directories (backup serves as an insurance)

These steps are necessary because Dgraph's underlying data format could have changed, and reloading the backup avoids encoding incompatibilities.

## Post Installation

Now that Dgraph is up and running, to understand how to add and query data to Dgraph, follow [Query Language Spec]({{< relref "query-language/index.md">}}). Also, have a look at [Frequently asked questions]({{< relref "faq/index.md" >}}).

## Troubleshooting
Here are some problems that you may encounter and some solutions to try.

### Running OOM (out of memory)

During bulk loading of data, Dgraph can consume more memory than usual, due to high volume of writes. That's generally when you see the OOM crashes.

The recommended minimum RAM to run on desktops and laptops is 16GB. Dgraph can take up to 7-8 GB with the default setting `-stw_ram_mb` set to 4096; so having the rest 8GB for desktop applications should keep your machine humming along.

On EC2/GCE instances, the recommended minimum is 8GB. If you still continue to have Dgraph crash because of OOM, reduce the number of cores using `-cores`. This would decrease the performance of Dgraph and in-turn reduce the pace of memory growth. You can see the default numbers of cores used by running `dgraph -help`, next to `-cores` flag.

## See Also

* [Product Roadmap to v1.0](https://github.com/dgraph-io/dgraph/issues/1)
