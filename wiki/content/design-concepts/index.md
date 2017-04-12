+++
date = "2017-03-20T22:25:17+11:00"
title = "Design Concepts"
+++

## Concepts

### XID <-> UID
All the entities get assigned a unique 64 bit integer id, `UID`. If the entity has an external id `XID`,
we fingerprint the `XID` and store the corresponding `UID`. If the entity does not have an `XID`,
we only add a new `UID` to indicate that the `UID` is used. All the posting lists refer to entities
via their UIDs. Given UIDs are 8 byte integers, this provides for a very efficient data representation.
During data import, we require that each unique entity has a unique `UID`.

### Edges

Typical data format is RDF [NQuad](https://www.w3.org/TR/n-quads/) which is:

* `Subject, Predicate, Object, Label`, aka
* `Entity, Attribute, Other Entity / Value, Label`

Both the terminologies get used interchangeably in our code. Dgraph considers edges to be directional,
i.e. from `Subject -> Object`. This is the direction that the queries would be run.

{{% notice "tip" %}}Dgraph can automatically generate a reverse edge. If the user wants to run
queries in that direction, they would need to define the [reverse edge]({{< relref "query-language/index.md#reverse-edges" >}})
as part of the schema.{{% /notice %}}

Internally, the RDF NQuad gets parsed into this format.

```
type DirectedEdge struct {
  Entity      uint64
  Attr        string
  Value       []byte
  ValueType   uint32
  ValueId     uint64
  Label       string
  Lang 	      string
  Op          DirectedEdge_Op // Set or Delete
  Facets      []*facetsp.Facet
}
```

Note that irrespective of the input, both `Entity` and `Object/ValueId` get converted in `UID` format
as explained in [XID <-> UID]({{< relref "#xid-uid" >}}).

### Posting List
Conceptually, a posting list contains all the `DirectedEdges` corresponding to an `Attribute`, in the
following format:

```
Attribute: Entity -> sorted list of ValueId // Everything in uint64 representation.
```

So, for, e.g., if we're storing a list of friends, such as:

Entity | Attribute| ValueId
-------|----------|--------
Me     | friend   | person0
Me     | friend   | person1
Me     | friend   | person2
Me     | friend   | person3


Then a posting list `friend` would be generated. Seeking for `Me` in this PL
would produce a list of friends, namely `[person0, person1, person2, person3]`.

The big advantage of having such a structure is that we have all the data to do one join in one
Posting List. This means, one RPC to
the machine serving that Posting List would result in a join, without any further
network calls, reducing joins to lookups.

Implementation wise, a `Posting List` is a list of `Postings`. This is how they look in
[Protocol Buffers]({{< relref "#protocol-buffers" >}}) format.
```
message Posting {
  fixed64 uid = 1;
  bytes value = 2;
  enum ValType {
    DEFAULT = 0;
    BINARY = 1;
    INT = 2; // We treat it as int64.
    FLOAT = 3;
    BOOL = 4;
    DATE = 5;
    DATETIME = 6;
    GEO = 7;
    UID = 8;
    PASSWORD = 9;
    STRING = 10;

  }
  ValType val_type = 3;
  enum PostingType {
    REF=0;          // UID
    VALUE=1;        // simple, plain value
    VALUE_LANG=2;   // value with specified language
        // VALUE_TIMESERIES=3; // value from timeseries, with specified timestamp
  }
  PostingType posting_type = 4;
  bytes metadata = 5; // for VALUE_LANG: Language, for VALUE_TIMESERIES: timestamp, etc..
  string label = 6;
  uint64 commit = 7;  // More inclination towards smaller values.
  repeated facetsp.Facet facets = 8;

  // TODO: op is only used temporarily. See if we can remove it from here.
  uint32 op = 12;
}

message PostingList {
  repeated Posting postings = 1;
  bytes checksum = 2;
  uint64 commit = 3; // More inclination towards smaller values.
}
```

There is typically more than one Posting in a PostingList.

The RDF Label is stored as `label` in each posting.
{{% notice "warning" %}}We don't currently retrieve label via query -- but would use it in the future.{{% /notice %}}

### RocksDB
PostingLists are served via [RocksDB](http://rocksdb.org), given the latter provides enough
knobs to decide how much data should be served out of memory, SSD or disk.
Also, it supports bloom filters on keys, which makes random lookups efficient.

To allow RocksDB full access to memory to optimize for caches, we'll have
one RocksDB instance per machine. Each instance would contain all the
posting lists served by the machine.

Posting Lists get stored in RocksDB, in a key-value format, like so:
```
(Predicate, Subject) --> PostingList
```

### Group
A set of Posting Lists sharing the same `Predicate` constitute a group. Each server can serve
multiple distinct [groups]({{< relref "deploy/index.md#data-sharding" >}}).

A group config file is used to determine which server would serve what groups. In the future
versions, live Dgraph server would be able to move tablets around depending upon heuristics.

If a groups gets too big, it could be split further. In this case, a single `Predicate` essentially
gets divided across two groups.

```
  Original Group:
            (Predicate, Sa..z)
  After split:
  Group 1:  (Predicate, Sa..i)
  Group 2:  (Predicate, Sj..z)
```

Note that keys are sorted in RocksDB. So, the group split would be done in a way to maintain that
sorting order, i.e. it would be split in a way where the lexicographically earlier subjects would be
in one group, and the later in the second.

### Replication and Server Failure
Each group should typically be served by atleast 3 servers, if available. In the case of a machine
failure, other servers serving the same group can still handle the load in that case.

### New Server and Discovery
Dgraph cluster can detect new machines allocated to the [cluster]({{< relref "deploy/index.md#cluster" >}}),
establish connections, and transfer a subset of existing predicates to it based on the groups served
by the new machine.

### Write Ahead Logs
Every mutation upon hitting the database doesn't immediately make it on disk via RocksDB. We avoid
re-generating the posting list too often, because all the postings need to be kept sorted, and it's
expensive. Instead, every mutation gets logged and synced to disk via append only log files called
`write-ahead logs`. So, any acknowledged writes would always be on disk. This allows us to recover
from a system crash, by replaying all the mutations since the last write to `Posting List`.

### Mutations
In addition to being written to `Write Ahead Logs`, a mutation also gets stored in memory as an
overlay over immutable `Posting list` in a mutation layer. This mutation layer allows us to iterate
over `Posting`s as though they're sorted, without requiring re-creating the posting list.

When a posting list has mutations in memory, it's considered a `dirty` posting list. Periodically,
we re-generate the immutable version, and write to RocksDB. Note that the writes to RocksDB are
asynchronous, which means they don't get flushed out to disk immediately, but that wouldn't lead
to data loss on a machine crash. When `Posting lists` are initialized, write-ahead logs get referred,
and any missing writes get applied.

Every time we regenerate a posting list, we also write the max commit log timestamp that was
included -- this helps us figure out how long back to seek in write-ahead logs when initializing
the posting list, the first time it's brought back into memory.

### Note on Transactions
Dgraph does not support transactions at this point. A mutation can be composed of multiple
[Edges]({{< relref "#edges">}}), each edge might belong to a different
[Posting List]({{< relref "#posting-list" >}}). Dgraph acquires `RWMutex` locks at a posting list
level. It DOES NOT acquire locks across multiple posting lists.

What this means for writes is that some edges would get written before others, and so any reads
which happen while a mutation is going on would read partially committed data. However, there's a
guarantee of [**durability**](https://en.wikipedia.org/wiki/ACID#Durability). When a mutation
succeeds, any successive reads will read the updated data in its entirety.

On the other hand, if a mutation fails, it's up to the application author to clean the partially
written data or retry with the right logic. The response from Dgraph would make it clear which edges
weren't written so that the application author can set the right edges for retry.

**Ways around this limitation**

* You should determine if your reads need to have transactional [atomicity](https://en.wikipedia.org/wiki/ACID#Atomicity).
Unless you are dealing with financial data, this might not be the case.
* To ensure atomicity, you could have only one edge (RDF NQuads) per mutation. This would mean more
network calls back and forth between client and server which would affect your write throughput, but
would help make the failure handling logic easier.

### Versioning
Broadly speaking, there're two kinds of data stored in Dgraph. One is relationship data; other is value data.
```
 Me friend person0    [Relation]
 Me name "Константи́н" [Value]
```

The way Dgraph stores [Posting List]({{< relref "#posting-list" >}}), the relationship data gets
converted to UIDs and stored in a sorted list of increasing uint64 ids to allow for efficient
traversal, lookups, and intersection.

Versioning involves writing deltas and reading them to generate the final state. This isn't as
effective when you're dealing with millions of UIDs and want to do the operations mentioned above.
It would be too slow and memory-consuming to re-generate the long posting lists for every read operation.

For those reasons, **versioning wouldn't be supported for relationship data.**

On the other hand, we only expect one value per `subject-predicate-language` composite. This allows
us to store many versions of this value in the same posting list, without having to do any
regeneration. So, we can potentially support versioning of value data.

{{% notice "warning" %}}Value data versioning is under consideration, and not yet implemented.{{% /notice %}}

### Queries

Let's understand how query execution works, by looking at an example.

```
me(id: m.abcde) {
  pred_A
  pred_B {
    pred_B1
    pred_B2
  }
  pred_C {
    pred_C1
    pred_C2 {
      pred_C21
   }
  }
}
```

Let's assume we have 3 server instances, and instance id = 2 receives this query. These are the steps:

* Determine the UID of provided XID, in this case `m.abcde` using fingerprinting. Say the UID = u.
* Send queries to look up keys = `pred_A, u`, `pred_B, u`, and `pred_C, u`. These predicates could
belong to 3 different groups, served by potentially different servers. So, this would typically
incur at max 3 network calls (equal to number of predicates at this step).
* The above queries would return back 3 list of ids or value. The result of `pred_B` and `pred_C`
would be converted into queries for `pred_Bi` and `pred_Ci`.
* `pred_Bi` and `pred_Ci` would then cause at max 4 network calls, depending upon where these
predicates are located. The keys for `pred_Bi` for e.g. would be `pred_Bi, res_pred_Bk`, where
res_pred_Bk = list of resulting ids from `pred_B, u`.
* Looking at `res_pred_C2`, you'll notice that this would be a list of lists aka list matrix. We
merge these list of lists into a sorted list with distinct elements to form the query for `pred_C21`.
* Another network call depending upon where `pred_C21` lies, and this would again give us a list of
list ids / value.

If the query was run via HTTP interface `/query`, this subgraph gets converted into JSON for
replying back to the client. If the query was run via [gRPC](https://www.grpc.io/) interface using
the language [clients]({{< relref "clients/index.md" >}}), the subgraph gets converted to
[protocol buffer](https://developers.google.com/protocol-buffers/) format, and returned to client.

### Network Calls
Compared to RAM or SSD access, network calls are slow.
Dgraph minimizes the number of network calls required to execute queries. As explained above, the
data sharding is done based on `predicate`, not `entity`. Thus, even if we have a large set of
intermediate results, they'd still only increase the payload of a network call, not the number of
network calls itself. In general, the number of network calls done in Dgraph is directly proportional
to the number of predicates in the query, or the complexity of the query, not the number of
intermediate or final results.

In the above example, we have eight predicates, and so including a call to convert to UID, we'll
have at max nine network calls. The total number of entity results could be in millions.

### Worker
In Queries section, you noticed how the calls were made to query for `(predicate, uids)`. All those
network calls / local processing are done via workers. Each server exposes a
[gRPC](https://www.grpc.io) interface, which can then be called by the query processor to retrieve data.

### Worker Pool
Worker Pool is just a pool of open TCP connections which can be reused by multiple goroutines.
This avoids having to recreate a new connection every time a network call needs to be made.

### Protocol Buffers
All data in Dgraph that is stored or transmitted is first converted into byte arrays through
serialization using [Protocol Buffers](https://developers.google.com/protocol-buffers/). When
the result is to be returned to the user, the protocol buffer object is traversed, and the JSON
object is formed.

## Minimizing network calls explained

To explain how Dgraph minimizes network calls, let's start with an example query we should be able
to run.

*Find all posts liked by friends of friends of mine over the last year, written by a popular author X.*

### SQL/NoSQL
In a distributed SQL/NoSQL database, this would require you to retrieve a lot of data.

Method 1:

* Find all the friends (~ 338 [friends](http://www.pewresearch.org/fact-tank/2014/02/03/6-new-facts-about-facebook/</ref>)).
* Find all their friends (~ 338 * 338 = 40,000 people).
* Find all the posts liked by these people over the last year (resulting set in millions).
* Intersect these posts with posts authored by person X.

Method 2:

* Find all posts written by popular author X over the last year (possibly thousands).
* Find all people who liked those posts (easily millions) `result set 1`.
* Find all your friends.
* Find all their friends `result set 2`.
* Intersect `result set 1` with `result set 2`.

Both of these approaches would result in a lot of data going back and forth between database and
application; would be slow to execute, or would require you to run an offline job.

### Dgraph
This is how it would run in Dgraph:

* Node X contains posting list for predicate `friends`.
* Seek to caller's userid in Node X **(1 RPC)**. Retrieve a list of friend uids.
* Do multiple seeks for each of the friend uids, to generate a list of friends of friends uids. `result set 1`
* Node Y contains posting list for predicate `posts_liked`.
* Ship result set 1 to Node Y **(1 RPC)**, and do seeks to generate a list of all posts liked by
result set 1. `reult set 2`
* Node Z contains posting list for predicate `author`.
* Ship result set 2 to Node Z **(1 RPC)**. Seek to author X, and generate a list of posts authored
by X. `result set 3`
* Intersect the two sorted lists, `result set 2` and `result set 3`. `result set 4`
* Node N contains names for all uids.
* Ship `result set 4` to Node N **(1 RPC)**, and convert uids to names by doing multiple seeks. `result set 5`
* Ship `result set 5` back to caller.

In 4-5 RPCs, we have figured out all the posts liked by friends of friends, written by popular author X.

This design allows vast scalability, and yet consistent production level latencies,
to support running complicated queries requiring deep joins.

## RAFT

This section aims to explain the RAFT consensus algorithm in simple terms. The idea is to give you
just enough to make you understand the basic concepts, without going into explanations about why it
works accurately. For a detailed explanation of RAFT, please read the original thesis paper by
[Diego Ongaro](https://github.com/ongardie/dissertation).

### Term
Each election cycle is considered a **term**, during which there is a single leader
*(just like in a democracy)*. When a new election starts, the term number is increased. This is
straightforward and obvious but is a critical factor for the accuracy of the algorithm.

In rare cases, if no leader could be elected within an `ElectionTimeout`, that term can end without
a leader.

### Server States
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

### Communication
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

### Log Entries
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

### Voting
Each server persists its current term and vote, so it doesn't end up voting twice in the same term.
On receiving a `RequestVote` RPC, the server denies its vote if its log is more up-to-date than the
candidate. It would also deny a vote, if a minimum `ElectionTimeout` hasn't passed since the last
<tt>Heartbeat</tt> from the leader. Otherwise, it gives a vote and resets its `ElectionTimeout` timer.

Up-to-date property of logs is determined as follows:

* Term number comparison
* Index number or log length comparison

{{% notice "tip" %}}To understand the above sections better, you can see this
[interactive visualization](http://thesecretlivesofdata.com/raft).{{% /notice %}}

### Cluster membership
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

### Log Compaction
One of the ways to do this is snapshotting. As soon as the state machine is synced to disk, the
logs can be discarded.

### Clients
Clients must locate the cluster to interact with it. Various approaches can be used for discovery.

A client can randomly pick up any server in the cluster. If the server isn't a leader, the request
should be rejected, and the leader information passed along. The client can then re-route it's query
to the leader. Alternatively, the server can proxy the client's request to the leader.

When a client first starts up, it can register itself with the cluster using `RegisterClient` RPC.
This creates a new client id, which is used for all subsequent RPCs.

### Linearizable Semantics

Servers must filter out duplicate requests. They can do this via session tracking where they use
the client id and another request UID set by the client to avoid reprocessing duplicate requests.
RAFT also suggests storing responses along with the request UIDs to reply back in case it receives
a duplicate request.

Linearizability requires the results of a read to reflect the latest committed write.
Serializability, on the other hand, allows stale reads.

### Read-only queries

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
