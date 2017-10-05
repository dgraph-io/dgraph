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

### Nightly

Nightly builds from Dgraph master branch at https://github.com/dgraph-io/dgraph are available from https://nightly.dgraph.io.  To install run:

```sh
curl https://nightly.dgraph.io -sSf | bash
```

The Docker version is available as _master_.  Pull and run with:

```sh
docker pull dgraph/dgraph:master
```

#### Run using installed binary

Run dgraphzero
```sh
dgraphzero -w zw
```

Run dgraph
```sh
dgraph --memory_mb 2048 --peer 127.0.0.1:8888
```

#### Run using Docker


Run dgraphzero
```sh
mkdir -p ~/dgraph
docker run -it -p 8080:8080 -p 9080:9080 -v ~/dgraph:/dgraph --name dgraph dgraph/dgraph:master dgraphzero -w zw
```

Run dgraph
```
docker exec -it dgraph dgraph --bindall=true --memory_mb 2048 -peer 127.0.0.1:8888
```

{{% notice "note" %}}All the usual cautions about nightly builds apply: the feature set is not stable and may change (even daily) and the nightly build may contain bugs. {{% /notice %}}

### Building from Source

Make sure you have [Go](https://golang.org/dl/) (version >= 1.8) installed.

After installing Go, run
```
# This should install the following binaries in your $GOPATH/bin: dgraph, dgraph-live-loader, and dgraph-bulk-loader.
go get -u github.com/dgraph-io/dgraph/...
```

The binaries are located in `cmd/dgraph`, `cmd/dgraph-live-loader`, and
`cmd/dgraph-bulk-loader`. If you get errors related to `grpc` while building them, your
`go-grpc` version might be outdated. We don't vendor in `go-grpc`(because it
causes issues while using the Go client). Update your `go-grpc` by running.
```
go get -u google.golang.org/grpc
```


## Endpoints

### Dgraph

On its http port, a running Dgraph instance exposes a number of service endpoints.

* `/` Browser UI and query visualization.
* `/query` receive queries and respond in JSON.
* `/share`
* `/health` HTTP status code 200 and "OK" message if worker is running, HTTP 503 otherwise.
<!-- * `/debug/store` backend storage stats.-->
* `/admin/shutdown` [shutdown]({{< relref "#shutdown">}}) a node.
* `/admin/export` take a running [export]({{< relref "#export">}}).

### Dgraphzero

Dgraphzero also exposes a http on (`--port` + 1). So if you ran `dgraphzero` using the default
value for port flag(`8888`), then it would have a HTTP server running on `8889`.

* `/state` Information about the nodes that are part of the cluster. Also contains information about
  size of predicates and groups they belong to.


## Running Dgraph

{{% notice "tip" %}}  All Dgraph tools have `--help`.  To view all the flags, run `dgraph --help`, it's a great way to familiarize yourself with the tools.{{% /notice %}}

Whether running standalone or in a cluster, each Dgraph instance relies on the following (if multiple instances are running on the same machine, instances cannot share these).

* A `p` directory.  This is where Dgraph persists the graph data as posting lists. (option `--p`, default: directory `p` where the instance was started)
* A `w` directory.  This is where Dgraph stores its write ahead logs. (option `--w`, default: directory `w` where the instance was started)
* The `p` and `w` directories must be different.
* A port for query, http client connections and other [endpoints]({{< relref "#endpoints">}}). (option `--port`, default: `8080` on `localhost`)
* A port for gRPC client connections. (option `--grpc_port`, default: `9080` on `localhost`)
* A port on which to run a worker node, used for Dgraph's communication between nodes. (option `--workerport`, default: `12345`)
* An address and port at which the node advertises its worker.  (option `--my`, default: `localhost:workerport`)
* Estimated memory dgraph can take. (option `--memory_mb, mandatory to specify, recommended value half of RAM size`)
* If you are running multiple dgraph instances on same machine for testing, you can use port offset to let dgraph use `default port + offset` instead of specifying all the ports. (option `--port_offset`)

{{% notice "note" %}}By default the server listens on `localhost` (the loopback address only accessible from the same machine).  The `--bindall=true` option binds to `0.0.0.0` and thus allows external connections. {{% /notice %}}

{{% notice "note" %}}Set max file descriptors to a high value like 10000 if you are going to load a lot of data.{{% /notice %}}
### Config
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

Dgraphzero can be configured with following options

```
# Replication factor (Number of replicas per data shard, count includes the original shard)
replicas
```

### Single Instance
A single instance can be run with default options, as in:

```sh
mkdir ~/dgraph # The folder where dgraph binary will create the directories it requires.
cd ~/dgraph 
dgraphzero -w wz
dgraph --memory_mb 2048 --peer "localhost:8888"
```

Or by specifying `p` and `w` directories, ports, etc.  If `dgraph-live-loader` is used, it must connect on the port exposing Dgraph services, as must the go client.

### Multiple instances

Dgraph is a truly distributed graph database - not a master-slave replication of one datastore.  Dgraph shards by predicate, not by node.  Running in a cluster shards and replicates predicates across the cluster, queries can be run on any node and joins are handled over the distributed data.

As well as the requirements for [each instance]({{< relref "#running-dgraph">}}), to run Dgraph effectively in a cluster, it's important to understand how sharding and replication work.

* Dgraph stores data per predicate (not per node), thus the unit of sharding and replication is predicates.
* To shard the graph, predicates are assigned to groups and each node in the cluster serves a singe group.
* Each node in a cluster stores only the predicates for the groups it is assigned to.
* If multiple cluster nodes server the same group, the data for that group is replicated

A query is resolved locally for predicates the node stores and via distributed joins for predicates stored on other nodes.

Note also:

* dgraphzero stores information about the cluster.
* Whenver a new machine is brought up it is assigned a group based on replication factor. If replication factor is 1 then each node will serve different group. If replciation factor is 2 and you launch 4 machines then first two machines would server group 1 and next two machines would server group 2.
* dgraphzero monitors the space occupied by predicates in each group and moves them around to rebalance the cluster.

#### Data sharding

* dgraphzero assigns predicates to group. At this point dgraphzero doesn't know what might be the growth rate of the data so the predicate is assigned to the group which asks first.
* Data for reverse edges and index are always stored along with the predicate.
* dgraphzero tries to rebalance the cluster based on the disk usage in each group. If dgraphzero detects an imbalance then dgraphzero would try to move a predicate along with index and reverse edges to a node which has less disk usage.


#### Running the Cluster

Each machine in the cluster must be started with a unique ID (option `--idx`) and address of dgraphzero server. Each machine must also satisfy the data directory and port requirements for a [single instance]({{< relref "#running-dgraph">}}).


To run a cluster, begin by bringing up dgraphzero.

```
$ dgraphzero -w wz  --bindall=true

# This will bring up dgraphzero using the default 8888 for grpc and 8889 for http.
```

We recommend running three instances of dgraphzero for high availability.
```
$ dgraphzero -w wz1 --peer "localhost:8888" --port 8890 --bindall=true -idx 2

# This will bring up dgraphzero using the default 8890 for grpc and 8891 for http.
```


{{% notice "note" %}} The `--bindall=true` option is required when running on multiple machines, otherwise the node's port and workerport will be bound to localhost and not be accessible over a network. {{% /notice %}}

Bring up dgraph nodes.

```
# specify dgraphzero's grpc port's address as peer address.
$ dgraph --idx 3 --peer "<ip address>:<workerport>" --my "ip-address-others-should-access-me-at" --bindall=true --memory_mb=2048
```

The new servers will automatically detect each other by communicating with dgraphzero and establish connections to each other if replication is greater than 1. Dgraphzero would assign groups to the new nodes.

{{% notice "note" %}}To have RAFT consensus work correctly, each group must be served by an odd number of Dgraph instances.{{% /notice %}}

You could start loading all the data into one of the dgraph nodes and dgraphzero would automatically rebalance the data for you.

#### Cluster Checklist

In setting up a cluster be sure the check the following.

* Is each dgraph instance in the cluster [set up correctly]({{< relref "#running-dgraph">}})?
* Will each instance be accessible to all peers on `workerport`?
* Does each node have a unique ID on startup?
* Has `--bindall=true` been set for networked communication?
* Is dgraphzero brought up first?


<!---
### In EC2

To make provisioning an EC2 Dgraph cluster quick, the following script installs and starts a Dgraph node, opens ports on the host machine and enters Dgraph into systemd so that the node will restart if the machine is rebooted.

We recommend an Ubuntu 16.04 AMI on a machine with at least 16GB of memory.

The i3 instances with SSD drives have the right hardware set up to run Dgraph (and its underlying key value store Badger) for performance environments.

After provisioning an Ubuntu machine the following scripts installs and brings up Dgraph.



## Docker
--->

## Bulk Data Loading

There are two different tools that can be used for bulk data loading:

- `dgraph-live-loader`
- `dgraph-bulk-loader` (will be available in v0.8.3)

{{% notice "note" %}} both tools only accepts gzipped, RDF NQuad/Triple data.
Data in other formats must be converted [to
this](https://www.w3.org/TR/n-quads/).{{% /notice %}}

### `dgraph-live-loader`

The `dgraph-live-loader` binary is a small helper program which reads RDF NQuads from a gzipped file, batches them up, creates mutations (using the go client) and shoots off to Dgraph. It's not the only way to run mutations.  Mutations could also be run from the command line, e.g. with `curl`, from the UI, by sending to `/query` or by a program using a [Dgraph client]({{< relref "clients/index.md" >}}).

`dgraph-live-loader` correctly handles splitting blank nodes across multiple batches and creating `xid` [edges for RDF URIs]({{< relref "query-language/index.md#external-ids" >}}) (option `-x`).

`dgraph-live-loader` checkpoints the loaded rdfs in the c directory by default. On restart it would automatically resume from the last checkpoint. If you want to load the whole data again, you need to delete the checkpoint directory.

```sh
$ dgraph-live-loader --help # To see the available flags.

# Read RDFs from the passed file, and send them to Dgraph on localhost:9080.
$ dgraph-live-loader -r <path-to-rdf-gzipped-file>

# Read RDFs and a schema file and send do Dgraph running at given address
$ dgraph-live-loader -r <path-to-rdf-gzipped-file> -s <path-to-schema-file> -d <dgraph-server-address:port>

# For example to load goldendata with the corresponding schema and convert URI to xid.
$ dgraph-live-loader -r github.com/dgraph-io/benchmarks/data/goldendata.rdf.gz -s github.com/dgraph-io/benchmarks/data/goldendata.schema -x
```

### `dgraph-bulk-loader`

{{% notice "note" %}}
This tool will become available in v0.8.3.
{{% /notice %}}

`dgraph-bulk-loader` serves a similar purpose to `dgraph-live-loader`, but can only be used
while dgraph is offline for the initial population. It cannot run on an
existing dgraph instance.

`dgraph-bulk-loader` is *considerably faster* than `dgraph-live-loader`, and is the recommended
way to perform the initial import of large datasets into dgraph.

You can [read some technical details](https://blog.dgraph.io/post/bulkloader/)
about the bulkloader on the blog.

Flags can be used to control the behaviour and performance characteristics of
the bulk loader. The following are from the output of `dgraph-bulk-loader
--help`:

```sh
Usage of dgraph-bulk-loader:
  -block int
        Block profiling rate.
  -cleanup_tmp
        Clean up the tmp directory after the loader finishes. Setting this to false allows the bulk loader can be re-run while skipping the map phase. (default true)
  -expand_edges
        Generate edges that allow nodes to be expanded using _predicate_ or expand(...). Disable to increase loading speed. (default true)
  -http string
        Address to serve http (pprof). (default "localhost:8080")
  -j int
        Number of worker threads to use (defaults to the number of logical CPUs) (default 4)
  -l string
        Location to write the lease file. (default "LEASE")
  -map_shards int
        Number of map output shards. Must be greater than or equal to the number of reduce shards. Increasing allows more evenly sized reduce shards, at the expense of increased memory usage. (default 1)
  -mapoutput_mb int
        The estimated size of each map file output. Increasing this increases memory usage. (default 64)
  -out string
        Location to write the final dgraph data directories. (default "out")
  -r string
        Directory containing *.rdf or *.rdf.gz files to load.
  -reduce_shards int
        Number of reduce shards. This determines the number of dgraph instances in the final cluster. Increasing this potentially decreases the reduce stage runtime by using more parallelism, but increases memory usage. (default 1)
  -s string
        Location of schema file to load.
  -shufflers int
        Number of shufflers to run concurrently. Increasing this can improve performance, and must be less than or equal to the number of reduce shards. (default 1)
  -skip_map_phase
        Skip the map phase (assumes that map output files already exist).
  -tmp string
        Temp directory used to use for on-disk scratch space. Requires free space proportional to the size of the RDF file and the amount of indexing used. (default "tmp")
  -version
        Prints the version of dgraph-bulk-loader.
```

We'll run through a complete example of loading a data set from start to
finish, using the *golden data* set from dgraph's
[benchmarks](https://github.com/dgraph-io/benchmarks).

Start with a fresh directory, then obtain the RDFs and schema.

```sh
$ wget https://github.com/dgraph-io/benchmarks/blob/master/data/goldendata.rdf.gz?raw=true -O goldendata.rdf.gz
$ wget https://raw.githubusercontent.com/dgraph-io/benchmarks/master/data/goldendata.schema
```
{{% notice "note" %}}
For bigger datasets and machines with many cores, gzip
decoding can be a bottleneck. Performance improvements can be obtained by
first splitting the RDFs up into many `.rdf.gz` files (e.g. 256MB each).
{{% /notice %}}

The next step is to run the bulk loader. First, you need to determine the
number of dgraph instances you want in your cluster. You should set the number
of reduce shards to this number. You will also need to set the number of map
shards to at least this number (a higher number helps the bulk loader evenly
distribute predicates between the reduce shards). For this example, we'll use
2 reduce shards and 4 map shards.

There are many different options,
look at the `--help` flag for details. For this tutorial, we only care about a
few options.

```sh
$ dgraph-bulk-loader -r=goldendata.rdf.gz -s=goldendata.schema -map_shards=4 -reduce_shards=2
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

You will now have some additional data in your directory.

The `LEASE` file indicates the UID lease that should be given to the new
dgraph cluster. The `p` directories are the posting list directories the
dgraph instances in the cluster will use. They contain all of the edges
required to run the dgraph cluster.

```sh
$ ls -l
total 11960
-rw-r--r-- 1 petsta petsta 12222898 Oct  4 16:41 goldendata.rdf.gz
-rw-r--r-- 1 petsta petsta       74 Oct  4 16:36 goldendata.schema
-rw-r--r-- 1 petsta petsta       10 Oct  4 16:42 LEASE
drwx------ 4 petsta petsta     4096 Oct  4 16:42 out
$ tree out
out
├── 0
│   └── p
│       ├── 000000.vlog
│       ├── 000001.sst
│       ├── 000002.sst
│       └── MANIFEST
└── 1
    └── p
        ├── 000000.vlog
        ├── 000001.sst
        └── MANIFEST

4 directories, 7 files
```

Now it's time to bring up `dgraphzero`. We need to give it an `id` and the
UID lease as given by the bulk loader.
```sh
$ mkdir zero
$ cd zero
$ dgraphzero -idx 1 -lease $(cat ../LEASE)
```
`dgraphzero` will stay in the foreground, so you'll need to open new
terminals for the next steps.

Now to start the dgraph instances. We'll start two, one for each output
directory from the bulk loader. We need to specify several things. First, they
need to know how to communicate with another peer. We can just use dgraphzero,
which listens on `localhost:8888`. Each dgraph instance also need to be assigned a
unique index. We also need to specify how much memory each dgraph instance
should use (this flag is required - but for a small data set such as *golden
data* we don't really care, so just use the minimum value of 1024 MB).
```sh
$ cd out/0
$ dgraph -peer=localhost:8888 -memory_mb=1024 -idx=10
```
For the second dgraph instance, the `-port_offset` flag prevents port conflicts
(since the default ports are used here).
```sh
$ cd out/1
$ dgraph -peer=localhost:8888 -memory_mb=1024 -idx=11 -port_offset=2000
```
Dgraphzero and the two dgraph instances should all now be running in the
foreground in separate terminals. Now you can connect to dgraph as normal and
do all of your usual queries and mutations. With multiple dgraph instances,
queries can be sent to any instance (in this case to either `localhost:8080` or
`localhost:10080`).
```sh
$ curl localhost:8080/query -XPOST -d '{
    pj_films(func:allofterms(name@en,"Peter Jackson")) {
        director.film (orderasc: name@en, first: 10) {
            name@en
        }
    }
}' | jq
```
```sh
{
  "data": {
    "pj_films": [
      {
        "director.film": [
          {
            "name@en": "The Lord of the Rings: The Return of the King"
          },
          {
            "name@en": "The Lovely Bones"
          },
          {
            "name@en": "Meet the Feebles"
          }
        ]
      }
    ]
  }
}
```

## Export

An export of all nodes is started by locally accessing the export endpoint of any server in the cluster.

```sh
$ curl localhost:8080/admin/export
```
{{% notice "warning" %}}This won't work if called from outside the server where dgraph is running.  Ensure that the port is set to the port given by `--port` on startup. {{% /notice %}}

This also works from a browser, provided the HTTP GET is being run from the same server where the Dgraph instance is running.

This triggers a export of all the groups spread across the entire cluster. Each server writes output in gzipped rdf to the export directory specified on startup by `--export`. If any of the groups fail, the entire export process is considered failed, and an error is returned.

{{% notice "note" %}}It is up to the user to retrieve the right export files from the servers in the cluster. Dgraph does not copy files  to the server that initiated the export.{{% /notice %}}

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
* delete (maybe do an export first) the `p` and `w` directories, then
* restart Dgraph.

## Upgrade Dgraph

<!--{{% notice "tip" %}}If you are upgrading from v0.7.3 you can modify the [schema file]({{< relref "query-language/index.md#schema">}}) to use the new syntax and give it to the dgraph-live-loader using the `-s` flag while reloading your data.{{% /notice %}}-->
{{% notice "note" %}}If you are upgrading from v0.7 please check whether you have the export api or get the latest binary for version v0.7.7 and use the export api. {{% /notice %}}

Doing periodic exports is always a good idea. This is particularly useful if you wish to upgrade Dgraph or reconfigure the sharding of a cluster. The following are the right steps safely export and restart.

- Start an [export]({{< relref "#export">}})
- Ensure it's successful
- Bring down the cluster
- Run Dgraph using new data directories.
- Reload the data via [bulk data loading]({{< relref "#bulk-data-loading" >}}).
- If all looks good, you can delete the old directories (export serves as an insurance)

These steps are necessary because Dgraph's underlying data format could have changed, and reloading the export avoids encoding incompatibilities.

## Post Installation

Now that Dgraph is up and running, to understand how to add and query data to Dgraph, follow [Query Language Spec]({{< relref "query-language/index.md">}}). Also, have a look at [Frequently asked questions]({{< relref "faq/index.md" >}}).

## Monitoring

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

## Troubleshooting
Here are some problems that you may encounter and some solutions to try.

### Running OOM (out of memory)

During bulk loading of data, Dgraph can consume more memory than usual, due to high volume of writes. That's generally when you see the OOM crashes.

The recommended minimum RAM to run on desktops and laptops is 16GB. Dgraph can take up to 7-8 GB with the default setting `-memory_mb` set to 4096; so having the rest 8GB for desktop applications should keep your machine humming along.

On EC2/GCE instances, the recommended minimum is 8GB. It's recommended to set `-memory_mb` to half of RAM size.

## See Also

* [Product Roadmap to v1.0](https://github.com/dgraph-io/dgraph/issues/1)
