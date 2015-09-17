## Overview

Dgraph is a distributed graph serving system, meant to be deployed in production,
and tackle user queries in real time. The aim of the system is to run complicated
joins by minimizing network calls required, and hence to keep end-to-end latency
low.

## Non-Goals
- Strict consistency would cause too much overhead for both reads and writes.
- The ability to query historical data for a given `Entity, Attribute`.

## Goals
- Distributed, low latency, meant for real time interaction at production loads.
- Dynamic scalability.
- Data redundancy. Handle machine crashes.
- Reliable eventual consistency.
- Support a general purpose data format, which could be used to serve both
sparse data and RDF schemas.
[Entity Attribute Value model](https://en.wikipedia.org/wiki/Entity%E2%80%93attribute%E2%80%93value_model)
*seems* more suited for this purpose.
<sub>TODO(manish): Research this a bit more</sub>

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
and network transfer.
Ref: [experiment](https://github.com/dgraph-io/experiments/tree/master/cproto)
- [RAFT via CoreOS](https://github.com/coreos/etcd/tree/master/raft), or possibly
[MultiRaft by Cockroachdb](http://www.cockroachlabs.com/blog/scaling-raft/)
- Use [tcp](http://golang.org/pkg/net/) for inter machine communication.
Ref: [experiment](https://github.com/dgraph-io/experiments/tree/master/vrpc)

## Internal representation
Internally, `Entity`, `Attribute` and `ValueId` are converted and stored in
`uint64` format. Integers are chosen over strings, because:
- uint64 is space efficient on disk, leading to smaller posting lists.
- uint64 is space efficient when sending over network,
leading to smaller network transmission times, compared to strings.
- uint64 comparison is quicker than string comparison.

So, just after query is received, it would be converted to internal uint64
representation. Once results are generated, they'd be converted back from
uint64 to strings. Note that the `Value` field is left as it is, as they
generally won't be passed around during joins.

For the purposes of conversion, a couple of internal sharded posting lists
would be used.

#### uint64 -> string
We store an internal sharded posting list for converting from `uint64`
representation to original `string` value. Once the query results
are computed utilizing internal `uint64` representation, this PL is
hit to retrieve back their original `string` representation.

Note that it's quite likely that this PL would have multiple shards, as
the number of unique ids grow in the system. Also, this PL would 
have to be kept in `strict consistency`, so we can avoid allocating
multiple `uint64`s to the same `string`Id.

#### string -> uint64
Instead of keeping another Posting List which points from `String -> Uint64`,
we could just use the already existant `Uint64 -> String` PL. This way
we could avoid synchronization issues between these posting lists, which
every query must hit, and have to be kept in `strict consistency`.

We could use such an algorithm:
```go
h := crc64.New(..)
io.WriteString(h, stringId)
rid := h.Sum64()
for {
	if pval, present := Uint64ToStringPL[rid]; present {
		if pval != stringId {
			rid += 1  // Increment sum by 1, until we find an empty slot.
								// Handle overflow.
			continue
		}
	} else {
		// New string id. Store in Uint64 to String Posting List.
		Uint64ToStringPL[rid] = stringId
	}
	break
}
```

## Sharded Posting Lists

#### Posting List (PL)
A posting list allows for super fast random lookups for `Attribute, Entity`.
It's implemented via RocksDB, given the latter provides enough
knobs to decide how much data should be served out of memory, ssd or disk.
In addition, it supports bloom filters on keys, which would help random lookups
required by Dgraph.

A posting list would be generated per `Attribute`.
In terms of RocksDB, this means each PL would correspond to a RocksDB database.
The key would be `Entity`,
and the value would be `sorted list of ValueIds`. Note that having sorted
lists make it really easy for doing intersects with other sorted lists.

```
Attribute: Entity -> sorted list of ValueId // Everything in uint64 format
```

Note that the above structure makes posting lists **directional**. You can do
Entity -> ValueId seeks, but not vice-versa.

**Example**: If we're storing a list of friends, such as:
```
Entity Attribute ValueId
---
Me friend person0
Me friend person1
Me friend person2
Me friend person3
```
Then a posting list `friend` would be generated. Seeking for `Me` in this PL
would produce a list of friends, namely `[person0, person1, person2, person3]`.

#### Why Sharded?
A single posting list can grow too big.
While RocksDB can serve data out of disk, it still requires RAM for bloom filters, which
allow for efficient random key lookups. If a single store becomes too big, both
it's data and bloom filters wouldn't fit in memory, and result in inefficient
data access. Also, more data = hot PL and longer initialization
time in case of a machine failure or PL inter-machine transfer.

To avoid such a scenario, we run compactions to split up posting lists, where
each such PL would then be renamed to:
`Attribute-MIN_ENTITY`.
A PL named as `Attribute-MIN_ENTITY` would contain all `keys > MIN_ENTITY`,
until either end of data, or beginning of another PL `Attribute-MIN_ENTITY_2`,
where `MIN_ENTITY_2 > MIN_ENTITY`.
Of course, most posting lists would start with just `Attribute`. The data threshold
which triggers such a split should be configurable.

Note that the split threshould would be configurable in terms of byte usage
(shard size), not frequency of access (or hotness of shard). Each join query
must still hit all the shards of a PL to retrieve entire dataset, so splits
based on frequency of access would stay the same. Moreover, shard hotness can
be addressed by increased replication of that shard. By default, each PL shard
would be replicated 3x. That would translate to 3 machines generally speaking.

**Caveat**:
Sharded Posting Lists don't have to be colocated on the same machine.
Hence, to do `Entity` seeks over sharded posting lists would require us to hit
multiple machines, as opposed to just one machine.

#### Terminology
Henceforth, a single Posting List shard would be referred to as shard. While
a Posting List would mean a collection of shards which together contain all
the data associated with an `Attribute`.

## Machine (Server)
Each machine can pick up multiple shards. For high availability even during
machine failures, multiple machines at random would hold replicas of each shard.
How many replicas are created per shard would be configurable, but defaults to 3.

However, only 1 out of the 3 or more machines holding a shard can do the writes. Which
machine would that be, depends on who's the master, determined via a
master election process. We'll be using CoreOS implementation of RAFT consensus
algorithm for master election.

Naturally, each machine would then be participating in a separate election process
for each shard located on that machine.
This could create a lot of network traffic, so we'll look into
using **MultiRaft by CockroachDB**.

#### Machine Failure
In case of a machine failure, the shards held by that machine would need to be
reassigned to other machines. RAFT could reliably inform us of the machine failure
or connection timeouts to that machine, in which case we could do the shard
reassignment depending upon which other machines have spare capacity.

#### New Machines & Discovery
Dgraph should be able to detect new machines allocated to the cluster, establish
connections to it, and reassign a subset of existing shards to it. `TODO(manish): Figure
out a good story for doing discovery.`

## Inter Machine Communication
We're using TCP directly for all inter machine communication. This was chosen
over TLS over TCP because of the significant performance difference, and the
expectation of a secure, access controlled environment within a data center,
which renders the overhead of TLS unnecessary.

Instead of using any custom library, we'll be using Go standard `net/rpc` package,
again based on [these benchmarks](https://github.com/dgraph-io/experiments/tree/master/vrpc).

## Backups and Snapshots
`TODO(manish): Fill this up`

