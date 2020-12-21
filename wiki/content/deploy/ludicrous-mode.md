+++
date = "2017-03-20T22:25:17+11:00"
title = "Ludicrous Mode"
weight = 13
[menu.main]
    parent = "deploy"
+++

Ludicrous mode is available in Dgraph v20.03.1 and later.

Ludicrous mode allows a Dgraph database to ingest data at an incredibly fast speed, but with fewer guarantees. In normal mode, Dgraph provides strong consistency.
In Ludicrous mode, Dgraph provides eventual consistency, so any mutation that succeeds should be available eventually. This means changes are applied more slowly during periods of peak data ingestion, and might not be immediately reflected in query results. 

{{% notice "note" %}}
If Dgraph is running in Ludicrous mode and crashes unexpectedly, there could be unapplied mutations which **will not** be picked up when Dgraph restarts.
{{% /notice %}}

Because Dgraph with Ludicrous mode enabled is eventually consistent, it is a good fit for any application where maximum performance is more important than strong real-time consistency and transactional guarantees (such as a social media app or game app).

### How do I enable it?

You can enable Ludicrous mode by setting the `--ludicrous_mode` config option on all Dgraph Zero and Dgraph Alpha nodes in a cluster.


### What does it do?

In this mode, Dgraph doesn't wait for mutations to be applied. When a mutation comes, it proposes the mutation to the cluster and as soon as the proposal reaches the other nodes, it returns the response right away. You don't need to send a commit request for mutations. It's equivalent of having `CommitNow` set automatically for all mutations. All the mutations are then sent to background workers which apply them continuously.

{{% notice "note" %}}
In Ludicrous mode, Dgraph does not sync writes to disk. If Dgraph crashes unexpectedly, there could be unapplied mutations which **will not** be picked up when Dgraph restarts.
{{% /notice %}}

### What is the trade-off?

As mentioned above, Ludicrous mode provides amazing speed at the cost of some guarantees.

It can be used to handle write-heavy operations when there is a time gap between queries and mutations, or when you are fine with potentially reading stale data.

{{% notice "note" %}}
There are no transactions in Ludicrous mode. That is, you cannot open a transaction, apply a mutation, and then decide to cancel the transaction. Every mutation request is committed to Dgraph.
{{% /notice %}}

### How does it assign timestamps? 

In normal mode, every transaction gets a `start timestamp`, and upon commit, it gets a `commit timestamp`. The transactions are flushed when they receive the `commit timestamp`.
In ludicrous mode, we assign a `commit timestamp` equal to the `start timestamp` and execute them instantly, which is why you don't need to send a commit request for mutation. One of the trades of using ludicrous mode is that the timestamps are assigned when they're ready to be committed, instead of following the arrival's order.

### Can you use Ludicrous mode in a highly-available (HA) cluster?

Yes, Ludicrous mode works with a cluster set up in a highly-available (HA) configuration.

### Can Ludicrous mode run in a cluster with multiple data shards?

Yes, Ludicrous mode works in a cluster set up with multiple data shards.

### How does Ludicrous mode handle concurrency?

Ludicrous mode now runs mutations concurrently to increase the speed of data
ingestion. This is enabled by default with `2000` total concurrent threads
available, but you can adjust the number of concurrent threads available using
the `--ludicrous_concurrency` configuration setting on the Alpha nodes in a
cluster.
