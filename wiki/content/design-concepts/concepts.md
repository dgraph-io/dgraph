+++
date = "2017-03-20T22:25:17+11:00"
title = "Concepts"
weight = 3
[menu.main]
    parent = "design-concepts"
+++

## Edges

Typical data format is RDF [N-Quad](https://www.w3.org/TR/n-quads/) which is:

* `Subject, Predicate, Object, Label`, aka
* `Entity, Attribute, Other Entity / Value, Label`

Both the terminologies get used interchangeably in our code. Dgraph considers edges to be directional,
i.e. from `Subject -> Object`. This is the direction that the queries would be run.

{{% notice "tip" %}}Dgraph can automatically generate a reverse edge. If the user wants to run
queries in that direction, they would need to define the [reverse edge]({{< relref "query-language/schema.md#reverse-edges" >}})
as part of the schema.{{% /notice %}}

Internally, the RDF N-Quad gets parsed into this format.

```
type DirectedEdge struct {
  Entity      uint64
  Attr        string
  Value       []byte
  ValueType   Posting_ValType
  ValueId     uint64
  Label       string
  Lang 	      string
  Op          DirectedEdge_Op // Set or Delete
  Facets      []*api.Facet
}
```

Note that irrespective of the input, both `Entity` and `Object/ValueId` get converted in `UID` format.

## Posting List
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
    DATETIME = 5;
    GEO = 6;
    UID = 7;
    PASSWORD = 8;
    STRING = 9;
    OBJECT = 10;
  }
  ValType val_type = 3;
  enum PostingType {
    REF=0;          // UID
    VALUE=1;        // simple, plain value
    VALUE_LANG=2;   // value with specified language
  }
  PostingType posting_type = 4;
  bytes lang_tag = 5; // Only set for VALUE_LANG
  string label = 6;
  repeated api.Facet facets = 9;

  // TODO: op is only used temporarily. See if we can remove it from here.
  uint32 op = 12;
  uint64 start_ts = 13;   // Meant to use only inmemory
  uint64 commit_ts = 14;  // Meant to use only inmemory
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

##  Badger
PostingLists are served via [Badger](https://github.com/dgraph-io/badger), given the latter provides enough
knobs to decide how much data should be served out of memory, SSD or disk.
Also, it supports bloom filters on keys, which makes random lookups efficient.

To allow Badger full access to memory to optimize for caches, we'll have
one Badger instance per machine. Each instance would contain all the
posting lists served by the machine.

Posting Lists get stored in Badger, in a key-value format, like so:
```
(Predicate, Subject) --> PostingList
```

## Group

Every Alpha server belongs to a particular group, and each group is responsible for serving a
particular set of predicates. Multiple servers in a single group replicate the same data to achieve
high availability and redundancy of data.

Predicates are automatically assigned to each group based on which group first receives the
predicate. By default periodically predicates can be moved around to different groups upon
heuristics to evenly distribute the data across the cluster. Predicates can also be moved manually
if desired.

In a future version, if a group gets too big, it could be split further. In this case, a single
`Predicate` essentially gets divided across two groups.

```
  Original Group:
            (Predicate, Sa..z)
  After split:
  Group 1:  (Predicate, Sa..i)
  Group 2:  (Predicate, Sj..z)
```

Note that keys are sorted in BadgerDB. So, the group split would be done in a way to maintain that
sorting order, i.e. it would be split in a way where the lexicographically earlier subjects would be
in one group, and the later in the second.

## Replication and Server Failure
Each group should typically be served by at least 3 servers, if available. In the case of a machine
failure, other servers serving the same group can still handle the load in that case.

## New Server and Discovery
Dgraph cluster can detect new machines allocated to the [cluster]({{< relref "deploy/cluster-setup.md" >}}),
establish connections, and transfer a subset of existing predicates to it based on the groups served
by the new machine.

## Write Ahead Logs
Every mutation upon hitting the database doesn't immediately make it on disk via BadgerDB. We avoid
re-generating the posting list too often, because all the postings need to be kept sorted, and it's
expensive. Instead, every mutation gets logged and synced to disk via append only log files called
`write-ahead logs`. So, any acknowledged writes would always be on disk. This allows us to recover
from a system crash, by replaying all the mutations since the last write to `Posting List`.

## Mutations

{{% notice "outdated" %}}
This section needs to be improved.
{{% /notice %}}

In addition to being written to `Write Ahead Logs`, a mutation also gets stored in memory as an
overlay over immutable `Posting list` in a mutation layer. This mutation layer allows us to iterate
over `Posting`s as though they're sorted, without requiring re-creating the posting list.

When a posting list has mutations in memory, it's considered a `dirty` posting list. Periodically,
we re-generate the immutable version, and write to BadgerDB. Note that the writes to BadgerDB are
asynchronous, which means they don't get flushed out to disk immediately, but that wouldn't lead
to data loss on a machine crash. When `Posting lists` are initialized, write-ahead logs get referred,
and any missing writes get applied.

Every time we regenerate a posting list, we also write the max commit log timestamp that was
included -- this helps us figure out how long back to seek in write-ahead logs when initializing
the posting list, the first time it's brought back into memory.

## Queries

Let's understand how query execution works, by looking at an example.

```
{
    me(func: uid(0x1)) {
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
}

```

Let's assume we have 3 Alpha instances, and instance id=2 receives this query. These are the steps:

* Send queries to look up keys = `pred_A, 0x1`, `pred_B, 0x1`, and `pred_C, 0x1`. These predicates could
belong to 3 different groups, served by potentially different Alpha servers. So, this would typically
incur at max 3 network calls (equal to number of predicates at this step).
* The above queries would return back 3 lists of UIDs or values. The result of `pred_B` and `pred_C`
would be converted into queries for `pred_Bi` and `pred_Ci`.
* `pred_Bi` and `pred_Ci` would then cause at max 4 network calls, depending upon where these
predicates are located. The keys for `pred_Bi`, for example, would be `pred_Bi, res_pred_Bk`, where
res_pred_Bk = list of resulting UIDs from `pred_B, u`.
* Looking at `res_pred_C2`, you'll notice that this would be a list of lists aka list matrix. We
merge these list of lists into a sorted list with distinct elements to form the query for `pred_C21`.
* Another network call depending upon where `pred_C21` lies, and this would again give us a list of
list UIDs / value.

If the query was run via HTTP interface `/query`, this subgraph gets converted into JSON for
replying back to the client. If the query was run via [gRPC](https://www.grpc.io/) interface using
the language [clients]({{< relref "clients/_index.md" >}}), the subgraph gets converted to
[protocol buffer](https://developers.google.com/protocol-buffers/) format and then returned to client.

## Network Calls
Compared to RAM or SSD access, network calls are slow.
Dgraph minimizes the number of network calls required to execute queries. As explained above, the
data sharding is done based on `predicate`, not `entity`. Thus, even if we have a large set of
intermediate results, they'd still only increase the payload of a network call, not the number of
network calls itself. In general, the number of network calls done in Dgraph is directly proportional
to the number of predicates in the query, or the complexity of the query, not the number of
intermediate or final results.

In the above example, we have eight predicates, and so including a call to convert to UID, we'll
have at max nine network calls. The total number of entity results could be in millions.

## Worker
In Queries section, you noticed how the calls were made to query for `(predicate, uids)`. All those
network calls / local processing are done via workers. Each server exposes a
[gRPC](https://www.grpc.io) interface, which can then be called by the query processor to retrieve data.

## Worker Pool
Worker Pool is just a pool of open TCP connections which can be reused by multiple goroutines.
This avoids having to recreate a new connection every time a network call needs to be made.

## Protocol Buffers
All data in Dgraph that is stored or transmitted is first converted into byte arrays through
serialization using [Protocol Buffers](https://developers.google.com/protocol-buffers/). When
the result is to be returned to the user, the protocol buffer object is traversed, and the JSON
object is formed.
