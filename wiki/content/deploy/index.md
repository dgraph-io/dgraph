+++
date = "2017-03-20T22:25:17+11:00"
title = "Deploy"
+++

This page talks about running Dgraph in a distributed fashion. This involves
running multiple instances of Dgraph, over multiple servers in a cluster.

{{% notice "tip" %}}
For a single server setup, recommended for new users, please see [Get Started]({{< relref "get-started/index.md" >}}) page.
{{% /notice %}}

## Install Dgraph
### Docker

```sh
docker pull dgraph/dgraph:latest

# You can test that it worked fine, by running:
docker run -it dgraph/dgraph:latest dgraph
```

### Automatic download

Running
```sh
curl https://get.dgraph.io -sSf | bash

# Test that it worked fine, by running:
dgraph
```
would install the `dgraph` binary into your system, along with assets required
for the UI.

### Manual download [optional]

If you don't want to follow the automatic installation method, you could manually download the appropriate tar for your platform from **[Dgraph releases](https://github.com/dgraph-io/dgraph/releases)**. After downloading the tar for your platform from Github, extract the binary to `/usr/local/bin` like so.

```sh
# For Linux
$ sudo tar -C /usr/local/bin -xzf dgraph-linux-amd64-VERSION.tar.gz

# For Mac
$ sudo tar -C /usr/local/bin -xzf dgraph-darwin-amd64-VERSION.tar.gz

# Test that it worked fine, by running:
dgraph
```

{{% notice "todo" %}}
Explain where to install UI assets.
{{% /notice %}}

### Nightly

Nightly builds from Dgraph master branch at https://github.com/dgraph-io/dgraph are available from https://nightly.dgraph.io.  To install run:

```sh
curl https://nightly.dgraph.io -sSf | bash
```

The Docker version is available as _master_.  Pull and run with:

```sh
docker pull dgraph/dgraph:master
```

## Simple cluster setup

### Understanding Dgraph cluster

Dgraph is a truly distributed graph database - not a master-slave replication of
universal dataset. It shards by predicate and replicates predicates across the
cluster, queries can be run on any node and joins are handled over the
distributed data.  A query is resolved locally for predicates the node stores,
and via distributed joins for predicates stored on other nodes.

For effectively running a Dgraph cluster, it's important to understand how
sharding, replication and rebalancing works.

**Sharding**

Dgraph colocates data per predicate (* P *, in RDF terminology), thus the
smallest unit of data is one predicate.  To shard the graph, one or many
predicates are assigned to a group.  Each node in the cluster serves a single
group. Dgraph zero assigns a group to each node.

**Shard rebalancing**

Dgraph zero tries to rebalance the cluster based on the disk usage in each
group. If Zero detects an imbalance, it would try to move a predicate along
with index and reverse edges to a group that has minimum disk usage. This can
make the predicate unavailable temporarily.

Zero would continuously try to keep the amount of data on each server even,
typically running this check on a 10-min frequency.  Thus, each additional
Dgraph server instance would allow Zero to further split the predicates from
groups and move them to the new node.

**Consistent Replication**

If `--replicas` flag is set to something greater than one, Zero would assign the
same group to multiple nodes. These nodes would then form a Raft group aka
quorum. Every write would be consistently replicated to the quorum. To achieve
consensus, its important that the size of quorum be an odd number. Therefore, we
recommend setting `--replicas` to 1, 3 or 5 (not 2 or 4). This allows 0, 1, or 2
nodes serving the same group to be down, respectively without affecting the
overall health of that group.

### Run directly on host

**Run dgraph zero**

```sh
dgraph zero --my=IPADDR:7080 --wal zw
```

The default Zero ports are 7080 for internal (Grpc) communication to other Dgraph
servers, and 8080 for HTTP communication with external processes. You can change
these ports by using `--port_offset` flag.

The `--my` flag is the connection that Dgraph servers would dial to talk to
zero. So, the port `7080` and the IP address must be visible to all the Dgraph servers.

For all the various flags available, run `dgraph zero --help`.

**Run dgraph server**

```sh
dgraph server --idx=2 --memory_mb=<typically half the RAM>
--my=IPADDR:7081 --zero=localhost:7080 --port_offset=1
```

**Dgraph server listens on port 7080 for internal, port 8080 for external HTTP and
port 9080 for external Grpc communication by default.** You can change that via
`--port_offset` flag.

Note that the `--idx` flag can be ommitted. Zero would then automatically assign
a unique ID to each Dgraph server, which Dgraph server persists in the write ahead log (wal) directory.

You can use `-p` and `-w` to change the storage location of data and WAL. For
all the various flags available, run `dgraph server --help`.

### Run using Docker

#### Docker Machine and Compose

Here we'll go through an example of deploying Dgraph Server and Zero on an AWS instance.
We will use [Docker Machine](https://docs.docker.com/machine/overview/). It is a tool that lets you install Docker Engine on virtual machines
and easily deploy applications.

* [Install Docker Machine](https://docs.docker.com/machine/install-machine/) on your machine.

* Now that you have Docker Machine installed, provisioning an instance on AWS is just one step
   away. You'll have to [configure your AWS credentials](http://docs.aws.amazon.com/sdk-for-java/v1/developer-guide/setup-credentials.html) for programatic access to the Amazon API.

* Create a new docker machine.

```sh
docker-machine create --driver amazonec2 aws01
```

Your output should look like

```sh
Running pre-create checks...
Creating machine...
(aws01) Launching instance...
...
...
Docker is up and running!
To see how to connect your Docker Client to the Docker Engine running on this virtual machine, run: docker-machine env aws01
```

The command would provision a `t2-micro` instance with a security group called `docker-machine`
(allowing inbound access on 2376 and 22). You can either edit the security group to allow inbound
access to `8080`, `9080` (default ports for Dgraph server) or you can provide your own security
group which allows inbound access on port 22, 2376 (required by Docker Machine), 8080 and 9080.

[Here](https://docs.docker.com/machine/drivers/aws/#options) is a list of full options for the `amazonec2` driver which allows you choose the
instance type, security group, AMI among many other
things.

{{% notice "tip" %}}Docker machine supports [other drivers](https://docs.docker.com/machine/drivers/gce/) like GCE, Azure etc.{{% /notice %}}

* Install and run Dgraph using docker-compose

Docker Compose is a tool for running multi-container Docker applications. You can follow the
instructions [here](https://docs.docker.com/compose/install/) to install it.

Copy the file below in a directory on your machine and name it `docker-compose.yml`.

```sh
version: "3"
services:
  zero:
    image: dgraph/dgraph:latest
    volumes:
      - /data:/dgraph
    restart: on-failure
    command: dgraph zero --port_offset -2000 --my=zero:5080
  server:
    image: dgraph/dgraph:latest
    volumes:
      - /data:/dgraph
    ports:
      - 8080:8080
      - 9080:9080
    restart: on-failure
    command: dgraph server --my=server:7080 --memory_mb=2048 --zero=zero:5080
```

{{% notice "note" %}}The config mounts `/data`(you could mount something else) on the instance to `/dgraph` within the
container for persistence.{{% /notice %}}

* Connect to the Docker Engine running on the machine.

Running `docker-machine env aws01` tells us to run the command below to configure
our shell.
```
eval $(docker-machine env aws01)
```
This configures our Docker client to talk to the Docker engine running on the AWS Machine.

Finally run the command below to start the Server and Zero.
```
docker-compose up -d
```
This would start 2 Docker containers, one running Dgraph Server and the other Zero on the same
machine. Docker would restart the containers in case there is any error.

You can look at the logs using `docker-compose logs`.

#### Manually

First, you'd want to figure out the host IP address. You can typically do that
via

```sh
ip addr  # On Arch Linux
ifconfig # On Ubuntu/Mac
```
We'll refer to the host IP address via `HOSTIPADDR`.

**Run dgraph zero**

```sh
mkdir ~/data # Or any other directory where data should be stored.

docker run -it -p 7080:7080 -p 8080:8080 -v ~/data:/dgraph dgraph/dgraph:latest dgraph zero --bindall=true --my=HOSTIPADDR:7080
```

**Run dgraph server**
```sh
mkdir ~/data # Or any other directory where data should be stored.

docker run -it -p 7081:7081 -p 8081:8081 -p 9081:9081 -v ~/data:/dgraph dgraph/dgraph:latest dgraph server -port_offset 1 --memory_mb=<typically half the RAM> --zero=HOSTIPADDR:7080 --my=HOSTIPADDR:7081 --idx <unique-id>
```


{{% notice "note" %}}
You can also use the `:master` tag when running docker image to get the nightly
build. Though, nightly isn't as thoroughly tested as a release, and we
recommend not using it.
{{% /notice %}}

### High-availability setup

In a high-availability setup, we need to run 3 or 5 replicas for Zero, and
similarly, 3 or 5 replicas for the server.
{{% notice "note" %}}
If number of replicas is 2K + 1, up to **K servers** can be down without any
impact on reads or writes.

Avoid keeping replicas to 2K (even number). If K servers go down, this would
block reads and writes, due to lack of consensus.
{{% /notice %}}

**Dgraph Zero**
Run three Zero instances, assigning a unique ID to each via `--idx` flag, and
passing the address of any healthy Zero instance via `--peer` flag.

To run three replicas for server, set `--replicas=3`. Every time a new Dgraph
server is added, Zero would check the existing groups and assign them to one,
which doesn't have three replicas.

**Dgraph Server**
Run as many Dgraph servers as you want. You can manually set `--idx` flag, or
you can leave that flag empty, and Zero would auto-assign an id to the server.
This id would get persisted in the write-ahead log, so be careful not to delete
it.

The new servers will automatically detect each other by communicating with
dgraphzero and establish connections to each other.

Typically, Zero would first attempt to replicate a group, by assigning a new
Dgraph server to run the same group as assigned to another. Once the group has
been replicated as per the `--replicas` flag, Zero would create a new group.

Over time, the data would be evenly split across all the groups. So, it's
important to ensure that the number of Dgraph servers is a multiple of the
replication setting. For e.g., if you set `--replicas=3` in Zero, then run three
Dgraph servers for no sharding, but 3x replication. Run six Dgraph servers, for
sharding the data into two groups, with 3x replication.

**Removing Dead Nodes**
If a replica goes down and can't be recovered, you can remove it and add a new node to the quorum. `/removeNode` endpoint on Zero can be used to remove the dead node (`/removeNode?id=3&group=2`).
{{% notice "note" %}}
Before using the api ensure that the node is down and ensure that it doesn't come back up ever again.

Remember to specify the `idx` flag while adding a new replica or else Zero might assign it a different group.
{{% /notice %}}

## Building from Source

Make sure you have [Go](https://golang.org/dl/) (version >= 1.8) installed.

After installing Go, run
```sh
# This should install dgraph binary in your $GOPATH/bin.

go get -u -v github.com/dgraph-io/dgraph/dgraph
```

If you get errors related to `grpc` while building them, your
`go-grpc` version might be outdated. We don't vendor in `go-grpc`(because it
causes issues while using the Go client). Update your `go-grpc` by running.
```sh
go get -u -v google.golang.org/grpc
```

## More about Dgraph

On its http port, a running Dgraph instance exposes a number of admin endpoints.

* `/` Browser UI and query visualization.
* `/health` HTTP status code 200 and "OK" message if worker is running, HTTP 503 otherwise.
* `/admin/shutdown` [shutdown]({{< relref "#shutdown">}}) a node.
* `/admin/export` take a running [export]({{< relref "#export">}}).

By default the server listens on `localhost` (the loopback address only accessible from the same machine).  The `--bindall=true` option binds to `0.0.0.0` and thus allows external connections.

{{% notice "tip" %}}Set max file descriptors to a high value like 10000 if you are going to load a lot of data.{{% /notice %}}

## More about Dgraph Zero

Dgraph Zero controls the Dgraph cluster. It automatically moves data between
different dgraph instances based on the size of the data served by each instance.

It is mandatory to run atleast one `dgraph zero` node before running any `dgraph server`.
Options present for `dgraph zero` can be seen by running `dgraph zero --help`.

* Zero stores information about the cluster.
* `--replicas` is the option that controls the replication factor. (i.e. number of replicas per data shard, including the original shard)
* Whenever a new machine is brought up it is assigned a group based on replication factor. If replication factor is 1 then each node will serve different group. If replication factor is 2 and you launch 4 machines then first two machines would server group 1 and next two machines would server group 2.
* Zero also monitors the space occupied by predicates in each group and moves them around to rebalance the cluster.

Like Dgraph, Zero also exposes HTTP on 8080 (+ any `--port_offset`). You can query it
to see useful information, like the following:

* `/state` Information about the nodes that are part of the cluster. Also contains information about
  size of predicates and groups they belong to.
* `/removeNode?id=idx&group=gid` Used to remove dead node from the quorum, takes node id and group id as query param.

## Config
{{% notice "note" %}}
Currently only valid for Dgraph server.
{{% /notice %}}

The command-line flags can be stored in a YAML file and provided via the `--config` flag.  For example:

```sh
# Folder in which to store exports.
export: export

# Fraction of dirty posting lists to commit every few seconds.
gentlecommit: 0.33

# RAFT ID that this server will use to join RAFT groups.
idx: 1

# Port to run server on. (default 8080)
port: 8080

# GRPC port to run server on. (default 9080)
grpc_port: 9080

# Port used by worker for internal communication.
workerport: 12345

# Estimated memory the process can take. Actual usage would be slightly more
memory_mb: 4096

# The ratio of queries to trace.
trace: 0.33

# Directory to store posting lists.
p: p

# Directory to store raft write-ahead logs.
w: w

# Debug mode for testing.
debugmode: false

# Address of dgraphzero
peer: localhost:8888
```

## TLS configuration
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


## Cluster Checklist

In setting up a cluster be sure the check the following.

* Is atleast one dgraphzero node running?
* Is each dgraph instance in the cluster [set up correctly]({{< relref "#running-dgraph">}})?
* Will each instance be accessible to all peers on 7080 (+ any port offset)?
* Does each node have a unique ID on startup?
* Has `--bindall=true` been set for networked communication?

## Fast Data Loading

There are two different tools that can be used for bulk data loading:

- `dgraph live`
- `dgraph bulk`

{{% notice "note" %}} Both tools only accepts gzipped, RDF NQuad/Triple data.
Data in other formats must be converted [to
this](https://www.w3.org/TR/n-quads/).{{% /notice %}}

### Live Loader

The `dgraph live` binary is a small helper program which reads RDF NQuads from a gzipped file, batches them up, creates mutations (using the go client) and shoots off to Dgraph.

Live loader correctly handles assigning unique IDs to blank nodes across multiple files, and persists them to disk to save memory and in case the loader was re-run.

```sh
$ dgraph live --help # To see the available flags.

# Read RDFs from the passed file, and send them to Dgraph on localhost:9080.
$ dgraph live -r <path-to-rdf-gzipped-file>

# Read RDFs and a schema file and send to Dgraph running at given address
$ dgraph live -r <path-to-rdf-gzipped-file> -s <path-to-schema-file> -d <dgraph-server-address:port>
```

### Bulk Loader

{{% notice "note" %}}
It's crucial to tune the bulk loaders flags to get good performance. See the
section below for details.
{{% /notice %}}

Bulk loader serves a similar purpose to the live loader, but can only be used
while dgraph is offline for the initial population. It cannot run on an
existing dgraph instance.

{{% notice "warning" %}}
Don't use bulk loader once Dgraph is up and running. Use it to import your
existing data into a new instance of Dgraph server.
{{% /notice %}}

Bulk loader is **considerably faster** than the live loader, and is the recommended
way to perform the initial import of large datasets into dgraph.

You can [read some technical details](https://blog.dgraph.io/post/bulkloader/)
about the bulk loader on the blog.

You need to determine the
number of dgraph instances you want in your cluster. You should set the number
of reduce shards to this number. You will also need to set the number of map
shards to at least this number (a higher number helps the bulk loader evenly
distribute predicates between the reduce shards). For this example, you could use
2 reduce shards and 4 map shards.

```sh
$ dgraph-bulk-loader -r=goldendata.rdf.gz -s=goldendata.schema --map_shards=4 --reduce_shards=2
{
        "RDFDir": "goldendata.rdf.gz",
        "SchemaFile": "goldendata.schema",
        "DgraphsDir": "out",
        "LeaseFile": "LEASE",
        "TmpDir": "tmp",
        "NumGoroutines": 4,
        "MapBufSize": 67108864,
        "ExpandEdges": true,
        "BlockRate": 0,
        "SkipMapPhase": false,
        "CleanupTmp": true,
        "NumShufflers": 1,
        "Version": false,
        "MapShards": 4,
        "ReduceShards": 2
}
MAP 01s rdf_count:219.0k rdf_speed:218.7k/sec edge_count:693.4k edge_speed:692.7k/sec
MAP 02s rdf_count:494.2k rdf_speed:247.0k/sec edge_count:1.596M edge_speed:797.7k/sec
MAP 03s rdf_count:749.4k rdf_speed:249.4k/sec edge_count:2.459M edge_speed:818.3k/sec
MAP 04s rdf_count:1.005M rdf_speed:250.8k/sec edge_count:3.308M edge_speed:826.1k/sec
MAP 05s rdf_count:1.121M rdf_speed:223.9k/sec edge_count:3.695M edge_speed:738.3k/sec
MAP 06s rdf_count:1.121M rdf_speed:186.6k/sec edge_count:3.695M edge_speed:615.3k/sec
MAP 07s rdf_count:1.121M rdf_speed:160.0k/sec edge_count:3.695M edge_speed:527.5k/sec
REDUCE 08s [22.68%] edge_count:837.9k edge_speed:837.9k/sec plist_count:450.2k plist_speed:450.2k/sec
REDUCE 09s [40.79%] edge_count:1.507M edge_speed:1.507M/sec plist_count:905.8k plist_speed:905.7k/sec
REDUCE 10s [79.91%] edge_count:2.953M edge_speed:1.476M/sec plist_count:1.395M plist_speed:697.3k/sec
REDUCE 11s [100.00%] edge_count:3.695M edge_speed:1.231M/sec plist_count:1.778M plist_speed:592.5k/sec
REDUCE 11s [100.00%] edge_count:3.695M edge_speed:1.182M/sec plist_count:1.778M plist_speed:568.8k/sec
Total: 11s
```

Once the data is generated, you can start the Dgraph servers by pointing their
`-p` directory to the output. If running multiple Dgraph servers, you'd need to
copy over the output shards into different servers.

```sh
$ cd out/i # i = shard number.
$ dgraph server -zero=localhost:7080 -memory_mb=1024
```

#### Performance Tuning

{{% notice "tip" %}}
We highly recommend [disabling swap
space](https://askubuntu.com/questions/214805/how-do-i-disable-swap) when
running Bulk Loader. It is better to fix the parameters to decrease memory
usage, than to have swapping grind the loader down to a halt.
{{% /notice %}}

Flags can be used to control the behaviour and performance characteristics of
the bulk loader. You can see the full list by running `dgraph bulk --help`. In
particular, **the flags should be tuned so that the bulk loader doesn't use more
memory than is available as RAM**. If it starts swapping, it will become
incredibly slow.

**In the map phase**, tweaking the following flags can reduce memory usage:

- The `--num_go_routines` flag controls the number of worker threads. Lowering reduces memory
  consumption.

- The `--mapoutput_mb` flag controls the size of the map output files. Lowering
  reduces memory consumption.

For bigger datasets and machines with many cores, gzip decoding can be a
bottleneck during the map phase. Performance improvements can be obtained by
first splitting the RDFs up into many `.rdf.gz` files (e.g. 256MB each). This
has a negligible impact on memory usage.

**The reduce phase** is less memory heavy than the map phase, although can still
use a lot.  Some flags may be increased to improve performance, *but only if
you have large amounts of RAM*:

- The `--reduce_shards` flag controls the number of resultant dgraph instances.
  Increasing this increases memory consumption, but in exchange allows for
higher CPU utilization.

- The `--map_shards` flag controls the number of separate map output shards.
  Increasing this increases memory consumption but balances the resultant
dgraph instances more evenly.

- The `--shufflers` controls the level of parallelism in the shuffle/reduce
  stage. Increasing this increases memory consumption.

# Export

An export of all nodes is started by locally accessing the export endpoint of any server in the cluster.

```sh
$ curl localhost:8080/admin/export
```
{{% notice "warning" %}}This won't work if called from outside the server where dgraph is running.
{{% /notice %}}

This also works from a browser, provided the HTTP GET is being run from the same server where the Dgraph instance is running.

This triggers a export of all the groups spread across the entire cluster. Each server writes output in gzipped rdf to the export directory specified on startup by `--export`. If any of the groups fail, the entire export process is considered failed, and an error is returned.

{{% notice "note" %}}It is up to the user to retrieve the right export files from the servers in the cluster. Dgraph does not copy files  to the server that initiated the export.{{% /notice %}}

# Shutdown

A clean exit of a single dgraph node is initiated by running the following command on that node.
{{% notice "warning" %}}This won't work if called from outside the server where dgraph is running.
{{% /notice %}}

```sh
$ curl localhost:8080/admin/shutdown
```

This stops the server on which the command is executed and not the entire cluster.

# Delete database

Individual triples, patterns of triples and predicates can be deleted as described in the [query languge docs]({{< relref "query-language/index.md#delete" >}}).

To drop all data, you could send a `DropAll` request via `/alter` endpoint.

Alternatively, you could:

* [stop Dgraph]({{< relref "#shutdown" >}}) and wait for all writes to complete,
* delete (maybe do an export first) the `p` and `w` directories, then
* restart Dgraph.

# Upgrade Dgraph

Doing periodic exports is always a good idea. This is particularly useful if you wish to upgrade Dgraph or reconfigure the sharding of a cluster. The following are the right steps safely export and restart.

- Start an [export]({{< relref "#export">}})
- Ensure it's successful
- Bring down the cluster
- Run Dgraph using new data directories.
- Reload the data via [bulk loader]({{< relref "#Bulk Loader" >}}).
- If all looks good, you can delete the old directories (export serves as an insurance)

These steps are necessary because Dgraph's underlying data format could have changed, and reloading the export avoids encoding incompatibilities.

# Post Installation

Now that Dgraph is up and running, to understand how to add and query data to Dgraph, follow [Query Language Spec]({{< relref "query-language/index.md">}}). Also, have a look at [Frequently asked questions]({{< relref "faq/index.md" >}}).

# Monitoring

Dgraph exposes metrics via `/debug/vars` endpoint in json format. Dgraph doesn't store the metrics and only exposes the value of the metrics at that instant. You can either poll this endpoint to get the data in your monitoring systems or install **[Prometheus](https://prometheus.io/docs/introduction/install/)**. Replace targets in the below config file with the ip of your dgraph instances and run prometheus using the command `prometheus -config.file my_config.yaml`.
```sh
scrape_configs:
  - job_name: "dgraph"
    metrics_path: "/debug/prometheus_metrics"
    scrape_interval: "2s"
    static_configs:
    - targets:
      - 172.31.9.133:8080
      - 172.31.15.230:8080
      - 172.31.0.170:8080
      - 172.31.8.118:8080
```

Install **[Grafana](http://docs.grafana.org/installation/)** to plot the metrics. Grafana runs at port 3000 in default settings. Create a prometheus datasource by following these **[steps](https://prometheus.io/docs/visualization/grafana/#creating-a-prometheus-data-source)**. Import **[grafana_dashboard.json](https://github.com/dgraph-io/benchmarks/blob/master/scripts/grafana_dashboard.json)** by following this **[link](http://docs.grafana.org/reference/export_import/#importing-a-dashboard)**.

# Troubleshooting
Here are some problems that you may encounter and some solutions to try.

## Running OOM (out of memory)

During bulk loading of data, Dgraph can consume more memory than usual, due to high volume of writes. That's generally when you see the OOM crashes.

The recommended minimum RAM to run on desktops and laptops is 16GB. Dgraph can take up to 7-8 GB with the default setting `-memory_mb` set to 4096; so having the rest 8GB for desktop applications should keep your machine humming along.

On EC2/GCE instances, the recommended minimum is 8GB. It's recommended to set `-memory_mb` to half of RAM size.

# See Also

* [Product Roadmap to v1.0](https://github.com/dgraph-io/dgraph/issues/1)
