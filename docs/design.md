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

## Internal Storage
Use [RocksDB](http://rocksdb.org/) for storing original data and posting lists.

## Sharded Posting Lists

#### What
A posting list would be generated per `Attribute`. The key would be `Entity`,
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
RocksDB can serve data out of disk, but still uses RAM for bloom filters, which
allow for efficient random key lookups. If a single store becomes too big, both
it's data and bloom filters wouldn't fit in memory, and result in inefficient
data access.

To avoid such a scenario, we run compactions to split up posting lists, where
each such posting list would then be renamed to:
`Attribute-MIN_VALUEID`.
Of course, most posting lists would start with just `Attribute`. The threshold
which triggers such a split should be configurable.

**Caveat**:
Sharded Posting Lists don't have to be colocated on the same machine.
Hence, to do `Entity` seeks over sharded posting lists would require us to hit
multiple machines, as opposed to just one machine.
