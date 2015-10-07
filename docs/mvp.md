## Overview

Dgraph is a distributed graph database, meant to tackle terabytes of data,
be deployed in production, and serve arbitrarily complex user queries in real
time, without predefined indexes. The aim of the system is to run complicated
joins by minimizing network calls required, and hence to keep end-to-end latency
low.

# MVP Design
This is an MVP design doc. This would only contain part of the functionality,
which can be pushed out of the door within a couple of months. This version
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
- [RAFT via CoreOS](https://github.com/coreos/etcd/tree/master/raft)
- Use [tcp](http://golang.org/pkg/net/) for inter machine communication.
Ref: [experiment](https://github.com/dgraph-io/experiments/tree/master/vrpc)

## Concepts / Technologies Skipped
- Strong consistency on the likes of Spanner / Cockroach DB. For this version,
we'll skip even eventual consistency. But, note that it is the long term plan
to make this a proper database, supporting strongly consistent writes.
- [MultiRaft by Cockroachdb](http://www.cockroachlabs.com/blog/scaling-raft/) would
be better aligned to support all the shards handled by one server. But it's also
more complex, and will have to wait for later versions.
- No automatic shard movement from one server to another.
- No distributed Posting Lists. One complete PL = one shard.

## Terminology

Term  | Definition | Link
:---: | --- | ---
Entity | Item being described | [link](https://en.wikipedia.org/wiki/Entity%E2%80%93attribute%E2%80%93value_model)
Attribute | A conceptual id (not necessary UUID) from a finite set of properties | [link](https://en.wikipedia.org/wiki/Entity%E2%80%93attribute%E2%80%93value_model)
Value | value of the attribute |
ValueId | If value is an id of another entity, `ValueId` is used to store that |
Posting List | A map with key = (Entity, Attribute) and value = (list of Values). This is how underlying data gets stored. |
Shard | Most granular data chunk that would be served by one server. In MVP, one shard = one complete posting list. |
Server | Machine connected to network, with local RAM and disk. Each server can serve multiple shards. |
Raft | Each shard would have multipe copies in different servers to distribute query load and improve availability. RAFT is the consensus algorithm used to determine leader among all copies of a shard. | [link](https://github.com/coreos/etcd/tree/master/raft)
Replica | Replica is defined as a non-leading copy of the shard after RAFT election, with the leading copy is defined as shard. |

