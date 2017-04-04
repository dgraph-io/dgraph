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

## Running Dgraph

{{% notice "tip" %}}To view all the flags, you can run `dgraph --help`. In fact, `--help` works on all Dgraph binaries and is a great way to familiarize yourself with the tools.{{% /notice %}}

### Config
As a helper utility, any of the flags provided to Dgraph binary on command-line can be stored in a YAML file and provided via `-config` flag. This is the default config located at `dgraph/cmd/dgraph/config.yaml`.

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
You could run a single instance like this

```sh
mkdir ~/dgraph # The folder where dgraph binary will create the directories it requires.
cd ~/dgraph
dgraph
```

### Multiple instances

#### Data sharding

You can readily shard Dgraph data by providing a groups config using the `-group_conf` flag. The data sharding is done based on predicate name.
Predicates are sharded over groups; where the same group could hold multiple predicates.
However, a single predicate shard would always lie completely within a single group.

{{% notice "note" %}}Shard calculation is done ''in-order of group id''. So, the lower group which applies to a predicate would be picked.{{% /notice %}}

The groups config syntax is as follows:

```
<shard-id>: comma separated list of predicate names or prefixes

# Last entry should be:
<shard-id>: fp % N + k, where N = number of shards you want, and k = starting shard id.
```

The default groups config used by Dgraph, when nothing is provided is:

```
$ cat cmd/dgraph/groups.conf
// Default formula for getting group where fp is the fingerprint of a predicate.
default: fp % 1 + 1

# fp % 1 is always zero. Thus, all data is located on group id 1.
```
{{% notice "warning" %}}Group id 0 is used to store membership information for the entire cluster. We don't take any snapshots of this group, so no data should be stored on group zero. [Related Github issue](https://github.com/dgraph-io/dgraph/issues/427){{% /notice %}}

Example of a valid groups.conf is:

```
// If * is specified prefix matching would be done, otherwise equality matching would be done.

1: _uid_, type.object.name
2: type.object.name*, film.performance.*

// Default formula for getting group where fp is the fingerprint of a predicate.
default: fp % 10 + 2
```

In the above spec, `_uid_`, and `type.object.name` predicates are going to be assigned to group 1.
Any predicate with prefix `type.object.name` and `film.performance.` will be assigned to group 2.
Note that `type.object.name` will belong to group 1 and not 2 despite matching both, because 1 is lower than 2.

Finally, all the rest of the predicates would be assigned by this formula: `fingerprint(predicate) % 10 + 2`.
They will occupy groups `[2, 3, 4, 5, 6, 7, 8, 9, 10, 11]`. ''Note that group 2 is overlapping between two rules.''

{{% notice "warning" %}}Once sharding spec is set, it **must not be changed** without bringing the cluster down. The same spec must be passed to all the nodes in the cluster.{{% /notice %}}

#### Cluster
To run a cluster, run a single server first. Every time you run a Dgraph instance, you should specify a comma-separated list of group ids it should handle, along with a server id unique across the cluster.

```
$ dgraph --groups "0,1" --idx 1 --my "ip-address-others-should-access-me-at"

# This instance would serve groups 0 and 1, using the default 8080 port for clients and 12345 for peers.
```

Now that one of the servers is up and running, you can point any number of new servers to this.

```
# Server handling only group 2.
$ dgraph --groups "2" --idx 3 --peer "<ip address>:<port>" --my "ip-address-others-should-access-me-at"

# Server handling groups 0, 1 and 2.
$ dgraph --groups "0,1,2" --idx 4 --peer "<ip address>:<port>" --my "ip-address-others-should-access-me-at"

# If running on the same server as other instances of Dgraph, do set the --port and the --workerport flags.
```

The new servers will automatically detect each other by communicating with the provided peer and establish connections to each other.
{{% notice "note" %}}To have consensus work correctly, each group must be served by an odd number of servers: 1, 3 or 5.{{% /notice %}}

{{% notice "tip" %}}Queries can be sent to any server in the cluster.{{% /notice %}}

## Bulk Data Loading
{{% notice "tip" %}}This is an optional step, only relevant if you have a lot of data that you need to quickly import into Dgraph.{{% /notice %}}

Dgraph loader binary is a small helper program which reads RDF NQuads from a gzipped file, batches them up, creates queries and shoots off to Dgraph. You don't need to use this program to load data, you can do the same thing by issuing batched queries via your own client. The code is [relatively straighforward](https://github.com/dgraph-io/dgraph/blob/master/cmd/dgraphloader/main.go#L54-L87).

{{% notice "note" %}}Dgraph only accepts gzipped data in RDF NQuad/Triple format. If you have data in some other format, you'll have to convert it [to this](https://www.w3.org/TR/n-quads/).{{% /notice %}}

If you just want to take Dgraph for a spin, we have both 1 million RDFs of golden data that we use for tests and 21 million RDFs from [Freebase film RDF data](https://github.com/dgraph-io/benchmarks/tree/master/data) that you can load up using this loader. Dgraphloader also accepts an
optional [schema]({{< relref "query-language/index.md#schema">}}).

```
$ dgraphloader --help # To see the available flags.

# The following would read RDFs from the passed file, and send them to Dgraph.
$ dgraphloader -r <path-to-rdf-gzipped-file> -s <path-to-schema-file>
$ dgraphloader -r <path-to-rdf-gzipped-file> -d <dgraph-server-address:port>
# For example to load goldendata with the corresponding schema.
$ dgraphloader -r github.com/dgraph-io/benchmarks/data/goldendata.rdf.gz -s github.com/dgraph-io/benchmarks/data/goldendata.schema
```

## Backup

You can take a backup of a running Dgraph cluster, by running the following command from any server in the cluster, like so:

```sh
$ curl localhost:8080/admin/backup
```
{{% notice "warning" %}}This won't work if called from outside the server where dgraph is running.{{% /notice %}}

You can do this via a browser as well, as long as the HTTP GET is being run from the same server, where Dgraph is running. This would trigger a backup of all the groups spread across the entire cluster. Each server would write the output in ''gzipped rdf'' format, in the backup directory as specified in Dgraph flags. If any of the groups fail, the entire backup process is considered failed, and an error would be output.

{{% notice "note" %}}It is up to the user to retrieve the right backups files from the servers in the cluster. Dgraph would not copy them over to the server where you ran the command from.{{% /notice %}}

## Shutdown

You can do a clean exit of a single dgraph node by running the following command on that server in the cluster, like so:

```sh
$ curl localhost:8080/admin/shutdown
```
{{% notice "warning" %}}This won't work if called from outside the server which is running dgraph.{{% /notice %}}

This would only stop the server on which the command is executed and not the entire cluster.

## Upgrade Dgraph

{{% notice "tip" %}}If you are upgrading from v0.7.3 you can modify the [schema file]({{< relref "query-language/index.md#schema">}}) to use the new syntax and give it to the dgraphloader using the `-s` flag.{{% /notice %}}

Doing periodic backups is always a good idea due to various reasons. This is particularly useful if you wish to upgrade Dgraph. The following are the right steps to switch over to a newer version of Dgraph.

- Run a [backup]({{< relref "#backup">}})
- Ensure it's successful
- Bring down the cluster
- Upgrade Dgraph binary
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
