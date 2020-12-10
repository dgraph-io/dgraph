+++
date = "2017-03-20T22:25:17+11:00"
title = "Cluster Setup"
weight = 3
[menu.main]
    parent = "deploy"
+++

{{% notice "tip" %}}
For a single server setup, recommended for new users, please see [Get Started]({{< relref "get-started/index.md" >}}) page.
{{% /notice %}}

## Understanding Dgraph cluster

Dgraph is a truly distributed graph database. It shards by predicate and
replicates predicates across the cluster, queries can be run on any node and
joins are handled over the distributed data. A query is resolved locally for
predicates the node stores, and using distributed joins for predicates stored on
other nodes.

To effectively running a Dgraph cluster, it's important to understand how
sharding, replication and rebalancing works.

### Sharding

Dgraph colocates data per predicate (* P *, in RDF terminology), thus the
smallest unit of data is one predicate. To shard the graph, one or many
predicates are assigned to a group. Each Alpha node in the cluster serves a
single group. Dgraph Zero assigns a group to each Alpha node.

### Shard rebalancing

Dgraph Zero tries to rebalance the cluster based on the disk usage in each
group. If Zero detects an imbalance, it will try to move a predicate along with
its indices to a group that has lower disk usage. This can make the predicate
temporarily read-only. Queries for the predicate will still be serviced, but any
mutations for the predicate will be rejected and should be retried after the
move is finished.

Zero would continuously try to keep the amount of data on each server even,
typically running this check on a 10-min frequency.  Thus, each additional
Dgraph Alpha instance would allow Zero to further split the predicates from
groups and move them to the new node.

### Consistent Replication

When starting Zero nodes, you can pass, to each one, the `--replicas` flag to assign
the same group to multiple nodes. The number passed to the `--replicas` flag
causes that Zero node to assign the same group to the specified number of nodes.
These nodes will then form a Raft group (or quorum), and every write will be
consistently replicated to the quorum.

To achieve consensus, it's important that the size of quorum be an odd number.
Therefore, we recommend setting `--replicas` to 1, 3 or 5 (not 2 or 4). This
allows 0, 1, or 2 nodes serving the same group to be down, respectively, without
affecting the overall health of that group.
