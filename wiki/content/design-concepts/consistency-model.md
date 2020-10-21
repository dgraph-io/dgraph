+++
date = "2017-03-20T22:25:17+11:00"
title = "Consistency Model"
weight = 2
[menu.main]
    parent = "design-concepts"
+++

[Update on Jan 15, 2020]

- Dgraph supports MVCC, Read Snapshots and Distributed ACID transactions.
- The transactions are cluster-wide (not key-only, or any other "crippled" version of them).
- Transactions are lockless. They don't block/wait on seeing pending writes by uncommitted transactions. Zero would choose to commit or abort them depending on conflicts.
- Transactions are based on Snapshot Isolation (not Serializable Snapshot Isolation), because conflicts are determined by writes (not reads).
- Dgraph hands out monotonically increasing timestamps (for transactions). Ergo, if any transaction Tx1 commits before Tx2 starts, then Ts_commit(Tx1) < Ts_start(Tx2).
- Any commit at Tc are guaranteed to be seen by a read at timestamp Tr by any client, if Tr > Tc.
- All reads are snapshots across the entire cluster, seeing all previously committed transactions in full.

---

{{% notice "outdated" %}}Sections below this one are outdated. You will find [Tour of Dgraph](https://dgraph.io/tour/) a much helpful resource.{{% /notice %}}

[Last updated: Mar 2018. This is outdated and is not how we do things anymore]
Basing it [on this
article](https://aphyr.com/posts/313-strong-consistency-models) by aphyr.

- **Sequential Consistency:** Different users would see updates at different times, but each user would see operations in order.

Dgraph has a client-side sequencing mode, which provides sequential consistency.

Here, let’s replace a “user” with a “client” (or a single process). In Dgraph, each client maintains a linearizable read map (linread map). Dgraph's data set is sharded into many "groups". Each group is a Raft group, where every write is done via a "proposal." You can think of a transaction in Dgraph, to consist of many group proposals.

The leader in Raft group always has the most recent proposal, while
replicas could be behind the leader in varying degrees. You can determine this
by just looking at the latest applied proposal ID. A leader's proposal ID would
be greater than or equal to some replicas' applied proposal ID.

`linread` map stores a group -> max proposal ID seen, per client. If a client's
last read had seen updates corresponding to proposal ID X, then `linread` map
would store X for that group. The client would then use the `linread` map to
inform future reads to ensure that the server servicing the request, has
proposals >= X applied before servicing the read. Thus, all future reads,
irrespective of which replica it might hit, would see updates for proposals >=
X. Also, the `linread` map is updated continuously with max seen proposal IDs
across all groups as reads and writes are done across transactions (within that
client).

In short, this map ensures that updates made by the client, or seen by the
client, would never be *unseen*; in fact, they would be visible in a sequential
order. There might be jumps though, for e.g., if a value X → Y → Z, the client
might see X, then Z (and not see Y at all).

- **Linearizability:** Each op takes effect atomically at some point between invocation and completion. Once op is complete, it would be visible to all.

Dgraph supports server-side sequencing of updates, which provides
linearizability. Unlike sequential consistency which provides sequencing per
client, this provide sequencing across all clients. This is necessary to make
transactions work across clients. Thus, once a transaction is committed,
it would be visible to all future readers, irrespective of client boundaries.

- **Causal consistency:** Dgraph does not have a concept of dependencies among transactions. So, does NOT order based on dependencies.
- **Serializable consistency:** Dgraph does NOT allow arbitrary reordering of transactions, but does provide a linear order per key.
