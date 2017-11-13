+++
title = "Dgraph compared to other databases"
+++

This page attempts to draw a comparison between Dgraph and other popular graph databases/datastores. The summaries that follow are brief descriptions that may help a person decide if Dgraph will suit their needs.

# Batch based
Batch based graph processing frameworks provide a very high throughput to do periodic processing of data. This is useful to convert graph data into a shape readily usable by other systems to then serve the data to end users.

## Pregel
* [Pregel](https://kowshik.github.io/JPregel/pregel_paper.pdf), is a system for large-scale graph processing by Google. You can think of it as equivalent to MapReduce/Hadoop.
* Pregel isn't designed to be exposed directly to users, i.e. run with real-time updates and execute arbitrary complexity queries. Dgraph is designed to be able to respond to arbitrarily complex user queries in low latency and allow user interaction.
* Pregel can be used along side Dgraph for complementary processing of the graph, to allow for queries which would take over a minute to run via Dgraph, or produce too much data to be consumed by clients directly.

---

# Database
Graph databases optimize internal data representation to be able to do graph operations efficiently.

## Neo4j
[Neo4j](https://neo4j.com/) is the most popular graph database according to [db-engines.com](http://db-engines.com/en/ranking/graph+dbms) and has been around since 2007. Dgraph is a much newer graph database built to scale to Google web scale and for serious production usage as the primary database.

### Language

Neo4j supports Cypher and Gremlin query language. Dgraph supports
[GraphQL+-]({{< relref "query-language/index.md#graphql">}}), a variation of
[GraphQL](https://facebook.github.io/graphql/), a query language created by
Facebook. As opposed to Cypher or Gremlin, which produce results in simple list
format, GraphQL allows results to be produced in a subgraph format, which has
richer semantics. Also, GraphQL supports schema validation which is useful to
ensure data correctness during both input and output.

While GraphQL is modern, Gremlin and Cypher are a lot more popular. Dgraph plans to support them after v1.0.

### Scalability

Neo4j runs on a single server. The enterprise version of Neo4j only runs
universal data replicas. As the data scales, this requires user to vertically
scale their servers. [Vertical scaling is expensive.][vert]

Dgraph has a distributed architecture. You can split your data among many Dgraph
servers to distribute it horizontally. As you add more data, you can just add
more commodity hardware to serve it. Dgraph bakes more performance features like
reducing network calls in a cluster and a highly concurrent execution of
queries, to achieve a high query throughput. Dgraph does consistent replication
of each shard, which makes it crash resilient, and protects users from server
downtime.

[vert]: https://blog.openshift.com/best-practices-for-horizontal-application-scaling/

### Transactions

Both systems provide ACID transactions. Neo4j supports ACID transactions in its
single server architecture. Dgraph, despite being a distributed and consistently
replicated system, supports ACID transactions with snapshot isolation.

### Replication

Neo4j's universal data replication is only available to users who purchase their
[enterprise license][neo4je]. At Dgraph, we consider horizontal scaling and
consistent replication the basic necessities of any application built today.
Dgraph not only would automatically shard your data, it would move data around
to rebalance these shards, so users achieve the best machine utilization and
query latency possible.

Dgraph is consistently replicated. Any read followed by a write would be visible
to the client, irrespective of which replica it hit. In short, we achieve
linearizable reads.

[neo4je]: https://neo4j.com/subscriptions/#editions

***For a more thorough comparison of Dgraph vs Neo4j, you can read our [blog](https://open.dgraph.io/post/benchmark-neo4j)***

---

# Datastore
Graph datastores act like a graph layer above some other SQL/NoSQL database to do the data management for them. This other database is the one responsible for backups, snapshots, server failures and data integrity.

## Cayley
* Both [Cayley](https://cayley.io/) and Dgraph are written primarily in Go language and inspired from different projects at Google.
* Cayley acts like a graph layer, providing a clean storage interface that could be implemented by various stores, for, e.g., PostGreSQL, RocksDB for a single machine, MongoDB to allow distribution. In other words, Cayley hands over data to other databases. While Dgraph uses [Badger](https://github.com/dgraph-io/badger), it assumes complete ownership over the data and tightly couples data storage and management to allow for efficient distributed queries.
* Cayley's design suffers from high fan-out issues. In that, if intermediate steps cause a lot of results to be returned, and the data is distributed, it would result in many network calls between Cayley and the underlying data layer. Dgraph's design minimizes the number of network calls, to reduce the number of servers it needs to touch to respond to a query. This design produces better and predictable query latencies in a cluster, even as cluster size increases.

***For a comparison of query and data loading benchmarks for Dgraph vs Cayley, you can read [Differences between Dgraph and Cayley](https://discuss.dgraph.io/t/differences-between-dgraph-and-cayley/23/3)***.
