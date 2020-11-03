+++
date = "2017-03-20T22:25:17+11:00"
title = "Ports Usage"
weight = 3
[menu.main]
    parent = "deploy"
+++

Dgraph cluster nodes use different ports to communicate over gRPC and HTTP. Users should pay attention while choosing these ports based on their topology and deployment-mode as each port needs different access security rules or firewall.

## Types of ports

- **gRPC-internal:** Port that is used between the cluster nodes for internal communication and message exchange.
- **gRPC-external:** Port that is used by Dgraph clients, Dgraph Live Loader , and Dgraph Bulk loader to access APIs over gRPC.
- **http-external:** Port that is used by clients to access APIs over HTTP and other monitoring & administrative tasks.

## Ports used by different nodes

 Dgraph Node Type |     gRPC-internal     | gRPC-external | HTTP-external
------------------|-----------------------|---------------|---------------
       zero       |      5080<sup>1</sup> | --Not Used--  |  6080<sup>2</sup>
       alpha      |      7080             |     9080      |  8080
       ratel      |  --Not Used--         | --Not Used--  |  8000


<sup>1</sup>: Dgraph Zero's gRPC-internal port is used for internal communication within the cluster. It's also needed for the [fast data loading]({{< relref "deploy/fast-data-loading.md" >}}) tools Dgraph Live Loader and Dgraph Bulk Loader.

<sup>2</sup>: Dgraph Zero's HTTP-external port is used for [admin]({{< relref "deploy/dgraph-zero.md" >}}) operations. Access to it is not required by clients.

Users have to modify security rules or open firewall ports depending up on their underlying network to allow communication between cluster nodes and between the Dgraph instances themselves and between Dgraph and a client. A general rule is to make *-external (gRPC/HTTP) ports wide open to clients and gRPC-internal ports open within the cluster nodes.

**Ratel UI** accesses Dgraph Alpha on the HTTP-external port (default localhost:8080) and can be configured to talk to remote Dgraph cluster. This way you can run Ratel on your local machine and point to a remote cluster. But if you are deploying Ratel along with Dgraph cluster, then you may have to expose 8000 to the public.

**Port Offset** To make it easier for user to setup the cluster, Dgraph defaults the ports used by Dgraph nodes and let user to provide an offset  (through command option `--port_offset`) to define actual ports used by the node. Offset can also be used when starting multiple zero nodes in a HA setup.

For example, when a user runs a Dgraph Alpha by setting `--port_offset 2`, then the Alpha node binds to 7082 (gRPC-internal), 8082 (HTTP-external) & 9082 (gRPC-external) respectively.

**Ratel UI** by default listens on port 8000. You can use the `-port` flag to configure to listen on any other port.

## HA Cluster Setup

In a high-availability setup, we need to run 3 or 5 replicas for Zero, and similarly, 3 or 5 replicas for Alpha.
{{% notice "note" %}}
If number of replicas is 2K + 1, up to **K servers** can be down without any impact on reads or writes.

Avoid keeping replicas to 2K (even number). If K servers go down, this would block reads and writes, due to lack of consensus.
{{% /notice %}}

### Dgraph Zero

Run three Zero instances, assigning a unique ID(Integer) to each via `--idx` flag, and
passing the address of any healthy Zero instance via `--peer` flag.

To run three replicas for the alphas, set `--replicas=3`. Every time a new
Dgraph Alpha is added, Zero would check the existing groups and assign them to
one, which doesn't have three replicas.

### Dgraph Alpha
Run as many Dgraph Alphas as you want. You can manually set `--idx` flag, or you
can leave that flag empty, and Zero would auto-assign an id to the Alpha. This
id would get persisted in the write-ahead log, so be careful not to delete it.

The new Alphas will automatically detect each other by communicating with
Dgraph zero and establish connections to each other. You can provide a list of
zero addresses to alpha using the `--zero` flag. Alpha will try to connect to
one of the zeros starting from the first zero address in the list. For example:
`--zero=zero1,zero2,zero3` where zero1 is the `host:port` of a zero instance.

Typically, Zero would first attempt to replicate a group, by assigning a new
Dgraph alpha to run the same group as assigned to another. Once the group has
been replicated as per the `--replicas` flag, Zero would create a new group.

Over time, the data would be evenly split across all the groups. So, it's
important to ensure that the number of Dgraph alphas is a multiple of the
replication setting. For e.g., if you set `--replicas=3` in Zero, then run three
Dgraph alphas for no sharding, but 3x replication. Run six Dgraph alphas, for
sharding the data into two groups, with 3x replication.
