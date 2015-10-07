## Overview

Dgraph is a distributed graph database, meant to tackle terabytes of data,
be deployed in production, and serve arbitrarily complex user queries in real
time, without predefined indexes. The aim of the system is to run complicated
joins by minimizing network calls required, and hence to keep end-to-end latency
low.

# MVP Design
This is an MVP design doc. This would only contain part of the functionality,
which can be pushed out of the door within a month. This version
would not enforce strong consistency, and might not be as distributed. Also,
shard movement from dead machines might not make a cut in this version.

## Data Storage Format

```go
type DirectedEdge struct {
	Entity string
	Attribute string
	Value interface{} // Store anything here
	ValueId string
	
	Source string // Optional. Used to store authorship, or web source
	Timestamp time.Time // Creation time. Useful for snapshotting.
}
```

## Technologies Used
- Use [RocksDB](http://rocksdb.org/) for storing original data and posting lists.
- Use [Cap'n Proto](https://capnproto.org/) for in-memory and on-disk representation,
- For this version, stick to doing everything on a single server. Possibly still
using TCP layer, to avoid complexities later.
- Possibly use a simple go mutex library for txn locking.
- Use UUID as entity id.
- Provide sorting in this version.

## Concepts / Technologies Skipped
- Distributed transactions the likes of Spanner / Cockroach DB.
For this version, the idea is to use mutex locks within a single process.
But, note that it is the long term plan
to make this a proper database, supporting strongly consistent writes.
- [RAFT via CoreOS](https://github.com/coreos/etcd/tree/master/raft)
- [MultiRaft by Cockroachdb](http://www.cockroachlabs.com/blog/scaling-raft/) would
be better aligned to support all the shards handled by one server. But it's also
more complex, and will have to wait for later versions.
- Use [tcp](http://golang.org/pkg/net/) for inter machine communication.
Ref: [experiment](https://github.com/dgraph-io/experiments/tree/master/vrpc)
- No automatic shard movement from one server to another.
- No distributed Posting Lists. One complete PL = one shard.
- Graph languages, like Facebook's GraphQL. For this version, just use some internal lingo
as the mode of communication.

## Terminology

Term  | Definition | Link
:---: | --- | ---
Entity | Item being described | [link](https://en.wikipedia.org/wiki/Entity%E2%80%93attribute%E2%80%93value_model)
Attribute | A conceptual id (not necessary UUID) from a finite set of properties | [link](https://en.wikipedia.org/wiki/Entity%E2%80%93attribute%E2%80%93value_model)
Value | value of the attribute |
ValueId | If value is an id of another entity, `ValueId` is used to store that |
Posting List | All the information related to one Attribute. A map with key = (Attribute, Entity) and value = (list of Values). This is how underlying data gets stored. |
Shard | Most granular data chunk that would be served by one server. In MVP, one shard = one complete posting list. In next versions, one shard might be a chunk of a posting list.|
Server | Machine connected to network, with local RAM and disk. Each server can serve multiple shards. |
Raft | Each shard would have multipe copies in different servers to distribute query load and improve availability. RAFT is the consensus algorithm used to determine leader among all copies of a shard. | [link](https://github.com/coreos/etcd/tree/master/raft)
Replica | Replica is defined as a non-leading copy of the shard after RAFT election, with the leading copy is defined as shard. |

## Simple design
- Have a posting list per Attribute.
- Store it in Rocks DB as (Attribute, Entity) -> (List of Values).
- One complete posting list = one shard.
- One mutex lock per shard.
- One single server, serving all shards.

## Write
- Convert the query into individual instructions with posting lists.
- Acquire write locks over all the posting lists.
- Do the reads.
- Do the writes.
- Release write locks.

## Read
- Figure out which posting list the data needs to be read from.
- Run the seeks, then repeat above step
- Consolidate and return.

## Sorting
- Would need to be done via bucketing of values.
- Sort attributes would have to be pre-defined to ensure correct posting
lists get generated.

## Goal
Doing this MVP would provide us with a working graph database, and
cement some of the basic concepts involved:
- Data storage and retrieval mechanisms (RocksDB, CapnProto).
- List intersections.
- List alterations to insert new items.

This design doc is deliberately kept simple and stupid.
At this stage, it's more important to pick a few concepts,
make progress on them and improve learning; instead of a multi-front
approch on a big and complicated design doc, which almost always changes
as we learn more. This way, we reduce the number of concepts we're working
on at the same time, and build up to the more complicated concepts that
are required in a full fledged strongly consistent and higly performant
graph databsae.
