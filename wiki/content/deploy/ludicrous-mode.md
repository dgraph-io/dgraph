+++
date = "2017-03-20T22:25:17+11:00"
title = "Ludicrous Mode"
weight = 13
[menu.main]
    parent = "deploy"
+++

Ludicrous mode is available in Dgraph v20.03.1 or later.

Ludicrous mode allows Dgraph database to ingest data at an incredibly fast speed. It differs from the normal mode as it provides fewer guarantees. In normal mode, Dgraph provides strong consistency. In ludicrous mode, Dgraph provides eventual consistency. In Ludicrous mode, any mutation which succeeds **might be available eventually**. **Eventually** means the changes will be applied later and might not be reflected in query results during data ingestion. If Dgraph crashes unexpectedly, there **might** be unapplied mutations which **will not** be picked up when Dgraph restarts. Dgraph with ludicrous mode enabled behaves as an eventually consistent system.


## How do I enable it?

You can enable Ludicrous mode by setting the `--ludicrous_mode` config option on all Dgraph Zero and Dgraph Alpha nodes in a cluster.


## What does it do?

In this mode, Dgraph doesn't wait for mutations to be applied. When a mutation comes, it proposes the mutation to the cluster and as soon as the proposal reaches the other nodes, it returns the response right away. You don't need to send a commit request for mutations. It's equivalent of having `CommitNow` set automatically for all mutations. All the mutations are then sent to background workers which keep applying them.

Also, Dgraph does not sync writes to disk. This increases throughput but may result in loss of unsynced writes in the event of hardware failure.


## What is the trade-off?

As mentioned in the section above, it provides amazing speed at the cost of some guarantees.

It can be used to handle write-heavy operations when there is a time gap between queries and mutations, or when you are fine with potentially reading stale data.

There are no transactions in ludicrous mode. That is, you cannot open a transaction, apply a mutation, and then decide to cancel the transaction. Every mutation request is committed to Dgraph.

## Can the cluster run with HA?

Yes, ludicrous mode works with the cluster set up in a highly-available (HA) configuration.

## Can the cluster run with multiple data shards?

Yes, ludicrous mode works with the cluster set up with multiple data shards.

## How does Ludicrous mode handle concurrency?

Ludicrous mode can now run mutations concurrently per predicate. This is enabled
with a default setting of 2000 concurrent threads, but you can adjust the number
of concurrent threads allowed using the `--ludicrous_concurrency` configuration
setting on all of the Alpha and Zero nodes in the cluster.
