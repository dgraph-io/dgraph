+++
date = "2017-03-20T22:25:17+11:00"
title = "RAFT"
weight = 5
[menu.main]
    parent = "design-concepts"
+++


This section aims to explain the RAFT consensus algorithm in simple terms. The idea is to give you
just enough to make you understand the basic concepts, without going into explanations about why it
works accurately. For a detailed explanation of RAFT, please read the original thesis paper by
[Diego Ongaro](https://github.com/ongardie/dissertation).

## Term
Each election cycle is considered a **term**, during which there is a single leader
*(just like in a democracy)*. When a new election starts, the term number is increased. This is
straightforward and obvious but is a critical factor for the accuracy of the algorithm.

In rare cases, if no leader could be elected within an `ElectionTimeout`, that term can end without
a leader.

## Server States
Each server in cluster can be in one of the following three states:

* Leader
* Follower
* Candidate

Generally, the servers are in leader or follower state. When the leader crashes or the communication
breaks down, the followers will wait for election timeout before converting to candidates. The
election timeout is randomized. This would allow one of them to declare candidacy before others.
The candidate would vote for itself and wait for the majority of the cluster to vote for it as well.
If a follower hears from a candidate with a higher term than the current (*dead in this case*) leader,
it would vote for it. The candidate who gets majority votes wins the election and becomes the leader.

The leader then tells the rest of the cluster about the result (<tt>Heartbeat</tt>
[Communication]({{< relref "#communication" >}})) and the other candidates then become followers.
Again, the cluster goes back into leader-follower model.

A leader could revert to being a follower without an election, if it finds another leader in the
cluster with a higher [Term]({{< relref "#term" >}})). This might happen in rare cases (network partitions).

## Communication
There is unidirectional RPC communication, from leader to followers. The followers never ping the
leader. The leader sends `AppendEntries` messages to the followers with logs containing state
updates. When the leader sends `AppendEntries` with zero logs, that's considered a
<tt>Heartbeat</tt>. Leader sends all followers <tt>Heartbeats</tt> at regular intervals.

If a follower doesn't receive <tt>Heartbeat</tt> for `ElectionTimeout` duration (generally between
150ms to 300ms), it converts it's state to candidate (as mentioned in [Server States]({{< relref "#server-states" >}})).
It then requests for votes by sending a `RequestVote` call to other servers. Again, if it gets
majority votes, candidate becomes a leader. At becoming leader, it then sends <tt>Heartbeats</tt>
to all other servers to establish its authority *(Cartman style, "Respect my authoritah!")*.

Every communication request contains a term number. If a server receives a request with a stale term
number, it rejects the request.

Raft believes in retrying RPCs indefinitely.

## Log Entries
Log Entries are numbered sequentially and contain a term number. Entry is considered **committed** if
it has been replicated to a majority of the servers.

On receiving a client request, the leader does four things (aka Log Replication):

* Appends and persists to its log.
* Issue `AppendEntries` in parallel to other servers.
* On majority replication, consider the entry committed and apply to its state machine.
* Notify followers that entry is committed so that they can apply it to their state machines.

A leader never overwrites or deletes its entries. There is a guarantee that if an entry is committed,
all future leaders will have it. A leader can, however, force overwrite the followers' logs, so they
match leader's logs *(elected democratically, but got a dictator)*.

## Voting
Each server persists its current term and vote, so it doesn't end up voting twice in the same term.
On receiving a `RequestVote` RPC, the server denies its vote if its log is more up-to-date than the
candidate. It would also deny a vote, if a minimum `ElectionTimeout` hasn't passed since the last
<tt>Heartbeat</tt> from the leader. Otherwise, it gives a vote and resets its `ElectionTimeout` timer.

Up-to-date property of logs is determined as follows:

* Term number comparison
* Index number or log length comparison

{{% notice "tip" %}}To understand the above sections better, you can see this
[interactive visualization](http://thesecretlivesofdata.com/raft).{{% /notice %}}

## Cluster membership
Raft only allows single-server changes, i.e. only one server can be added or deleted at a time.
This is achieved by cluster configuration changes. Cluster configurations are communicated using
special entries in `AppendEntries`.

The significant difference in how cluster configuration changes are applied compared to how typical
[Log Entries]({{< relref "#log-entries" >}}) are applied is that the followers don't wait for a
commitment confirmation from the leader before enabling it.

A server can respond to both `AppendEntries` and `RequestVote`, without checking current
configuration. This mechanism allows new servers to participate without officially being part of
the cluster. Without this feature, things won't work.

When a new server joins, it won't have any logs, and they need to be streamed. To ensure cluster
availability, Raft allows this server to join the cluster as a non-voting member. Once it's caught
up, voting can be enabled. This also allows the cluster to remove this server in case it's too slow
to catch up, before giving voting rights *(sort of like getting a green card to allow assimilation
before citizenship is awarded providing voting rights)*.


{{% notice "tip" %}}If you want to add a few servers and remove a few servers, do the addition
before the removal. To bootstrap a cluster, start with one server to allow it to become the leader,
and then add servers to the cluster one-by-one.{{% /notice %}}

## Log Compaction
One of the ways to do this is snapshotting. As soon as the state machine is synced to disk, the
logs can be discarded.

Snapshots are taken by default after 10000 Raft entries. This number can be adjusted using the
`dgraph alpha --snapshot_after` flag.

## Clients
Clients must locate the cluster to interact with it. Various approaches can be used for discovery.

A client can randomly pick up any server in the cluster. If the server isn't a leader, the request
should be rejected, and the leader information passed along. The client can then re-route it's query
to the leader. Alternatively, the server can proxy the client's request to the leader.

When a client first starts up, it can register itself with the cluster using `RegisterClient` RPC.
This creates a new client id, which is used for all subsequent RPCs.

## Linearizable Semantics

Servers must filter out duplicate requests. They can do this via session tracking where they use
the client id and another request UID set by the client to avoid reprocessing duplicate requests.
RAFT also suggests storing responses along with the request UIDs to reply back in case it receives
a duplicate request.

Linearizability requires the results of a read to reflect the latest committed write.
Serializability, on the other hand, allows stale reads.

## Read-only queries

To ensure linearizability of read-only queries run via leader, leader must take these steps:

* Leader must have at least one committed entry in its term. This would allow for up-to-dated-ness.
*(C'mon! Now that you're in power do something at least!)*
* Leader stores it's latest commit index.
* Leader sends <tt>Heartbeats</tt> to the cluster and waits for ACK from majority. Now it knows
that it's the leader. *(No successful coup. Yup, still the democratically elected dictator I was before!)*
* Leader waits for its state machine to advance to readIndex.
* Leader can now run the queries against state machine and reply to clients.

Read-only queries can also be serviced by followers to reduce the load on the leader. But this
could lead to stale results unless the follower confirms that its leader is the real leader(network partition).
To do so, it would have to send a query to the leader, and the leader would have to do steps 1-3.
Then the follower can do 4-5.

Read-only queries would have to be batched up, and then RPCs would have to go to the leader for each
batch, who in turn would have to send further RPCs to the whole cluster. *(This is not scalable
without considerable optimizations to deal with latency.)*

**An alternative approach** would be to have the servers return the index corresponding to their
state machine. The client can then keep track of the maximum index it has received from replies so far.
And pass it along to the server for the next request. If a server's state machine hasn't reached the
index provided by the client, it will not service the request. This approach avoids inter-server
communication and is a lot more scalable. *(This approach does not guarantee linearizability, but
should converge quickly to the latest write.)*
