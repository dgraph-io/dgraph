+++
date = "2017-03-20T22:25:17+11:00"
title = "Ports Usage"
weight = 3
[menu.main]
    parent = "deploy"
+++

Dgraph cluster nodes use a variety of ports to communicate over gRPC and HTTP.
Choose these ports carefully based on your topology and mode of deployment, as
this will impact the access security rules or firewall configurations required
for each port.

## Types of ports

Dgraph Alpha and Dgraph Zero nodes use a variety of gRPC and HTTP ports, as
follows:

- **gRPC-internal-private**: Used between the cluster nodes for internal
 communication and message exchange. Communication using these ports is
 TLS-encrypted.
- **gRPC-external-private**: Used by Dgraph Live Loader and Dgraph Bulk loader
 to access APIs over gRPC.
- **gRPC-external-public**: Used by Dgraph clients to access APIs in a session
 that can persist after a query.
- **HTTP-external-private**: Used for monitoring and administrative tasks.
- **HTTP-external-public:** Used by clients to access APIs over HTTP.

## Default ports used by different nodes

 Dgraph Node Type |  gRPC-internal-private | gRPC-external-private | gRPC-external-public | HTTP-external-private | HTTP-external-public
------------------|------------------------|-----------------------|----------------------|-----------------------|---------------------
       zero       |       5080<sup>1</sup> | 5080<sup>1</sup>      |                      | 6080<sup>2</sup>      |
       alpha      |       7080             |                       |     9080             |                       |    8080
       ratel      |                        |                       |                      |                       |    8000


<sup>1</sup>: Dgraph Zero uses port 5080 for internal communication within the
 cluster, and to support the [fast data loading]({{< relref "deploy/fast-data-loading.md" >}})
 tools: Dgraph Live Loader and Dgraph Bulk Loader.

<sup>2</sup>: Dgraph Zero uses port 6080 for
[administrative]({{< relref "deploy/dgraph-zero.md" >}}) operations. Dgraph
clients cannot access this port.

Users must modify security rules or open firewall ports depending upon their
underlying network to allow communication between cluster nodes, between the
Dgraph instances, and between Dgraph clients. In general, you should configure
the gRPC and HTTP `external-public` ports for open access by Dgraph clients,
and configure the gRPC-internal ports for open access by the cluster nodes.

**Ratel UI** accesses Dgraph Alpha on the `HTTP-external-public port` (which defaults to localhost:8080) and can be configured to talk to a remote Dgraph cluster. This
way you can run Ratel on your local machine and point to a remote cluster. But,
if you are deploying Ratel along with Dgraph cluster, then you may have to
expose port 8000 to the public.

**Port Offset** To make it easier for users to set up the cluster, Dgraph has
default values for the ports used by Dgraph nodes and lets users provide an
offset (using the command option `--port_offset`) to define actual ports used by
the node. Port offset can also be used when starting multiple Dgraph Zero nodes
in a high-availability (HA) setup.

For example, when a user runs Dgraph Alpha with the `--port_offset 2` setting,
then the Alpha node binds to port 7082 (`gRPC-internal-private`), 8082
(`HTTP-external-public`) and 9082 (`gRPC-external-public`), respectively.

**Ratel UI** by default listens on port 8000. You can use the `-port` flag to
configure it to listen on any other port.

## High Availability (HA) cluster configuration

In a HA cluster configuration, you should run three or five
replicas for the Zero node, and three or five replicas for the Alpha node.

{{% notice "note" %}}
If the number of replicas is 2N + 1, up to **N servers** can go offline without
any impact on reads or writes. The total number of Zero and Alpha node replicas
should be an odd number, as an even number of replicas will prevent consensus if
nodes go offline (which will impact reads and writes).
{{% /notice %}}

### Dgraph Zero

Run three Dgraph Zero instances, assigning a unique integer ID to each using the
`--idx` flag, and passing the address of any healthy Zero instance using the
`--peer` flag.

To run three replicas for the Alpha nodes, set `--replicas=3`. Each time a new
Alpha node is added, the Zero node will check the existing groups and assign
them as appropriate.

### Dgraph Alpha
You can run as many Dgraph Alpha nodes as you want. You can manually set the
`--idx` flag, or you can leave that flag empty, and the Zero node will
auto-assign an id to the Alpha node. This id persists in the write-ahead log, so
be careful not to delete it.

The new Alpha nodes will automatically detect each other by communicating with
Dgraph Zero and establish connections to each other. You can provide a list of
Zero node addresses to Alpha nodes using the `--zero` flag. The Alpha node will
try to connect to one of the Zero nodes starting from the first Zero node
address in the list. For example:
`--zero=zero1,zero2,zero3` where `zero1` is the `host:port` of a zero instance.

Typically, a Zero node would first attempt to replicate a group, by assigning a
new Alpha node to run the same group previously assigned to another. After the
group has been replicated per the `--replicas` flag, Dgraph Zero creates a new
group.

Over time, the data will be evenly split across all of the groups. So, it's
important to ensure that the number of Alpha nodes is a multiple of the
replication setting. For example, if you set `--replicas=3` in for a Zero node,
and then run three Alpha nodes for no sharding, but 3x replication. Or, if you
run six Alpha nodes, sharding the data into two groups, with 3x replication.
