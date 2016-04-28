# DGraph
**Scalable, Distributed, Low Latency, High Throughput Graph Database.**

![logo](https://img.shields.io/badge/status-alpha-red.svg)
[![Wiki](https://img.shields.io/badge/res-wiki-blue.svg)](https://github.com/dgraph-io/dgraph/wiki)
[![logo](https://img.shields.io/badge/Mailing%20List-dgraph-brightgreen.svg)](https://groups.google.com/forum/#!forum/dgraph)
[![Gitter](https://badges.gitter.im/Join%20Chat.svg)](https://gitter.im/dgraph-io/dgraph?utm_source=badge&utm_medium=badge&utm_campaign=pr-badge)

DGraph's goal is to provide [Google](https://www.google.com) production level scale and throughput,
with low enough latency to be serving real time user queries, over terabytes of structured data.
DGraph supports [GraphQL](http://graphql.org/) as query language, and responds in [JSON](http://www.json.org/).

The README is divided into these sections:
- [Current Status](#current-status)
- [Quick Testing](#quick-testing)
- [Installation](#installation)
- [Usage](#usage)
- [Queries and Mutations](#queries-and-mutations)
- [Contact](#contact)

## Current Status

*Check out [the demo at dgraph.io](http://dgraph.io).*

`Upcoming v0.3`
Track progress on [our Trello Board](https://trello.com/b/PF4nZ1vH).
Got questions or issues? Talk to us [via discuss](https://discuss.dgraph.io).

`Mar 2016 - Branch v0.2`
This is the first truly distributed version of DGraph.
Please see the [release notes here](https://discuss.dgraph.io/t/dgraph-v0-2-release/17).

`MVP launch - Dec 2015 - Branch v0.1`
This is a minimum viable product, alpha release of DGraph. **It's not meant for production use.**
This version is not distributed and support for GraphQL is partial.
[See the Roadmap](https://github.com/dgraph-io/dgraph/issues/1) for list of working and planned features.

Your feedback is welcome. Feel free to [file an issue](https://github.com/dgraph-io/dgraph/issues)
when you encounter bugs and to direct the development of DGraph.

There's an instance of DGraph running at http://dgraph.xyz, that you can query without installing DGraph.
This instance contains 21M facts from [Freebase Film Data](http://www.freebase.com/film).
See [Queries and Mutations below](#queries-and-mutations) for sample queries.
`curl dgraph.xyz/query -XPOST -d '{}'`


## Quick Testing

### Single instance via Docker
There's a docker image that you can readily use for playing with DGraph.
```
$ docker pull dgraph/dgraph:latest
# Setting a `somedir` volume on the host will persist your data.
$ docker run -t -i -v /somedir:/dgraph -p 80:8080 dgraph/dgraph:latest
```

Now that you're within the Docker instance, you can start the server.
```
$ mkdir /dgraph/m # Ensure mutations directory exists.
$ server --mutations /dgraph/m --postings /dgraph/p --uids /dgraph/u
```
There are some more options that you can change. Run `server --help` to look at them.

Run some mutations and query the server, like so:
```
# Make Alice follow Bob, and give them names.
$ curl localhost:80/query -X POST -d $'mutation { set {<alice> <follows> <bob> . \n <alice> <name> "Alice" . \n <bob> <name> "Bob" . }}'

# Now run a query to find all the people Alice follows 2 levels deep. The query would only result in 1 connection, Alice to Bob.
$ curl localhost:80/query -X POST -d '{me(_xid_: alice) { name _xid_ follows { name _xid_ follows {name _xid_ } } }}'

# Make Bob follow Greg.
$ curl localhost:80/query -X POST -d $'mutation { set {<bob> <follows> <greg> . \n <greg> <name> "Greg" .}}'

# The same query as above now would now show 2 connections, one from Alice to Bob, another from Bob to Greg.
$ curl localhost:80/query -X POST -d '{me(_xid_: alice) { name _xid_ follows { name _xid_ follows {name _xid_ } } }}'
```
Note how we can retrieve XIDs by using `_xid_` identifier.

### Multiple distributed instances
We have loaded 21M RDFs from Freebase Films data along with their names into 3 shards.
They're located in dgraph-io/benchmarks repository.
To use it, install [Git LFS first](https://git-lfs.github.com/).
I've found the Linux download to be the easiest way to install.
Note that this repository has over 1GB worth of data.
```
$ git clone https://github.com/dgraph-io/benchmarks.git
$ cd benchmarks/rocks
$ tar -xzvf uids.async.tar.gz -C $DIR
$ tar -xzvf postings.tar.gz -C $DIR
# You should now see directories p0, p1, p2 and uasync.final. The last directory name is unfortunate, but made sense at the time.
```
For quick testing, you can bring up 3 different processes of DGraph. You can of course, also set this up across multiple servers.
```
go build . && ./server --instanceIdx 0 --mutations $DIR/m0 --port 8080 --postings $DIR/p0 --workers ":12345,:12346,:12347" --uids $DIR/uasync.final --workerport ":12345" &
go build . && ./server --instanceIdx 1 --mutations $DIR/m1 --port 8082 --postings $DIR/p1 --workers ":12345,:12346,:12347" --workerport ":12346" &
go build . && ./server --instanceIdx 2 --mutations $DIR/m2 --port 8084 --postings $DIR/p2 --workers ":12345,:12346,:12347" --workerport ":12347" &
```
Now you can run any of the queries mentioned in [Test Queries](https://github.com/dgraph-io/dgraph/wiki/Test-Queries).
You can hit any of the 3 processes, they'll produce the same results.

`curl localhost:8080/query -XPOST -d '{}'`

## Installation
Best way to do this is to refer to [Dockerfile](Dockerfile), which has the most complete
instructions on getting the right setup.
All the instructions below are based on a Debian/Ubuntu system.

### Install Go 1.6
Download and install [Go 1.6 from here](https://golang.org/dl/).

### Install RocksDB
DGraph depends on [RocksDB](https://github.com/facebook/rocksdb) for storing posting lists.

```
# First install dependencies.
# For Ubuntu, follow the ones below. For others, refer to INSTALL file in rocksdb.
$ sudo apt-get update && apt-get install libgflags-dev libsnappy-dev zlib1g-dev libbz2-dev
$ git clone https://github.com/facebook/rocksdb.git
$ cd rocksdb
$ git checkout v4.2
$ make shared_lib
$ sudo make install
```

This would install RocksDB library in `/usr/local/lib`. Make sure that your `LD_LIBRARY_PATH` is correctly pointing to it.

```
# In ~/.bashrc
export LD_LIBRARY_PATH="/usr/local/lib"
```

### Install DGraph
Now get [DGraph](https://github.com/dgraph-io/dgraph) code. DGraph uses `glock` to fix dependency versions.
```
go get -v github.com/robfig/glock
go get -v github.com/dgraph-io/dgraph/...
glock sync github.com/dgraph-io/dgraph

# Optional
go test github.com/dgraph-io/dgraph/...
```

## Usage

### Distributed Bulk Data Loading
Let's load up data first. If you have RDF data, you can use that.
Or, there's [Freebase film rdf data here](https://github.com/dgraph-io/benchmarks).

Bulk data loading happens in 2 passes.

#### First Pass: UID Assignment
We first find all the entities in the data, and allocate UIDs for them.
You can run this either as a single instance, or over multiple instances.

Here we set number of instances to 2.
```
$ cd $GOPATH/src/github.com/dgraph-io/dgraph/server/uidassigner

# Run instance 0.
$ go build . && ./uidassigner --numInstances 2 --instanceIdx 0 --rdfgzips $BENCHMARK_REPO/data/rdf-films.gz,$BENCHMARK_REPO/data/names.gz --uids ~/dgraph/uids/u0

# And either later, or on another server, run instance 1.
$ go build . && ./uidassigner --numInstances 2 --instanceIdx 1 --rdfgzips $BENCHMARK_REPO/data/rdf-films.gz,$BENCHMARK_REPO/data/names.gz --uids ~/dgraph/uids/u1
```

Once the shards are generated, you need to merge them before the second pass. If you ran this as a single instance, merging isn't required.
```
$ cd $GOPATH/src/github.com/dgraph-io/dgraph/tools/merge
$ go build . && ./merge --stores ~/dgraph/uids --dest ~/dgraph/uasync.final
```
The above command would iterate over all the directories in `~/dgraph/uids`, and merge their data into one `~/dgraph/uasync.final`.
Note that this merge step is important if you're generating multiple uid intances, because all the loader instances need to have access to global uids list.

#### Second Pass: Data Loader
Now that we have assigned UIDs for all the entities, the data is ready to be loaded.

Let's do this step with 3 instances.
```
$ cd $GOPATH/src/github.com/dgraph-io/dgraph/server/loader
$ go build . && ./loader --numInstances 3 --instanceIdx 0 --rdfgzips $BENCHMARK_REPO/data/names.gz,$BENCHMARK_REPO/data/rdf-films.gz --uids ~/dgraph/uasync.final --postings ~/dgraph/p0
$ go build . && ./loader --numInstances 3 --instanceIdx 1 --rdfgzips $BENCHMARK_REPO/data/names.gz,$BENCHMARK_REPO/data/rdf-films.gz --uids ~/dgraph/uasync.final --postings ~/dgraph/p1
$ go build . && ./loader --numInstances 3 --instanceIdx 2 --rdfgzips $BENCHMARK_REPO/data/names.gz,$BENCHMARK_REPO/data/rdf-films.gz --uids ~/dgraph/uasync.final --postings ~/dgraph/p2
```
You can run these over multiple machines, or just one after another.

#### Loader performance
Loader is typically memory bound. Every mutation loads a posting list in memory, where mutations
are applied in layers above posting lists.
While loader doesn't write to disk every time a mutation happens, it does periodically
merge all the mutations to posting lists, and writes them to rocksdb which persists them.

There're 2 types of merging going on: Gentle merge, and Aggressive merge.
Gentle merging picks up N% of `dirty` posting lists, where N is currently 7, and merges them. This happens every 5 seconds.

Aggressive merging happens when the memory usage goes above `stw_ram_mb`.
When that happens, the loader would *stop the world*, start the merge process, and evict all posting lists from memory.
The more memory is available for loader to work with, the less frequently aggressive merging needs to be done, the faster the loading.

As a reference point, for instance 0 and 1, it took **11 minutes each to load 21M RDFs** from `rdf-films.gz` and `names.gz`
(from [benchmarks repository](https://github.com/dgraph-io/benchmarks/tree/master/data)) on
[n1-standard-4 GCE instance](https://cloud.google.com/compute/docs/machine-types)
using SSD persistent disk. Instance 2 took a bit longer, and finished in 15 mins. The total output including uids was 1.3GB.

Note that `stw_ram_mb` is based on the memory usage perceived by Golang. It currently doesn't take into account the memory usage by RocksDB. So, the actual usage is higher.

### Server
Now that the data is loaded, you can run the DGraph servers. To serve the 3 shards above, you can follow the [same steps as here](#multiple-distributed-instances).
Now you can run GraphQL queries over freebase film data like so:
```
curl localhost:8080/query -XPOST -d '{
	me(_xid_: m.06pj8) {
		type.object.name.en
		film.director.film {
			type.object.name.en
			film.film.starring {
				film.performance.character {
					type.object.name.en
				}
				film.performance.actor {
					type.object.name.en
					film.director.film {
						type.object.name.en
					}
				}
			}
			film.film.initial_release_date
			film.film.country
			film.film.genre {
				type.object.name.en
			}
		}
	}
}' > output.json
```
This query would find all movies directed by Steven Spielberg, their names, initial release dates, countries, genres, and the cast of these movies, i.e. characteres and actors playing those characters; and all the movies directed by these actors, if any.

The support for GraphQL is [very limited right now](https://github.com/dgraph-io/dgraph/issues/1).
You can conveniently browse [Freebase film schema here](http://www.freebase.com/film/film?schema=&lang=en).
There're also some schema pointers in [README](https://github.com/dgraph-io/benchmarks/blob/master/data/README.md).

#### Query Performance
With the [data loaded above](#loading-performance) on the same hardware,
it took **218ms to run** the pretty complicated query above the first time after server run.
Note that the json conversion step has a bit more overhead than captured here.
```json
{
  "server_latency": {
      "json": "37.864027ms",
      "parsing": "1.141712ms",
      "processing": "163.136465ms",
      "total": "202.144938ms"
  }
}
```

Consecutive runs of the same query took much lesser time (80 to 100ms), due to posting lists being available in memory.
```json
{
  "server_latency": {
    "json": "38.3306ms",
    "parsing": "506.708µs",
    "processing": "32.239213ms",
    "total": "71.079022ms"
	}
}
```

## Queries and Mutations
You can see a list of [sample queries here](https://discuss.dgraph.io/t/list-of-test-queries/22).
DGraph also supports mutations via GraphQL syntax.
Because GraphQL mutations don't contain complete data, the mutation syntax uses [RDF NQuad format](https://www.w3.org/TR/n-quads/).
```
mutation {
  set {
		<subject> <predicate> <objectid> .
		<subject> <predicate> "Object Value" .
		<subject> <predicate> "объект"@ru .
		_uid_:0xabcdef <predicate> <objectid> .
	}
}
```

You can batch multiple NQuads in a single GraphQL query.
DGraph would assume that any data in `<>` is an external id (XID),
and it would retrieve or assign unique internal ids (UID) automatically for these.
You can also directly specify the UID like so: `_uid_: 0xhexval` or `_uid_: intval`.

Note that a `delete` operation isn't supported yet.

In addition, you could couple a mutation with a follow up query, in a single GraphQL query like so.
```
mutation {
  set {
		<alice> <follows> <greg> .
	}
}
query {
	me(_xid_: alice) {
		follows
	}
}
```
The query portion is executed after the mutation, so this would return `greg` as one of the results.


## Contributing to DGraph
- Please see [this wiki page](https://discuss.dgraph.io/t/contributing-to-dgraph/20) for guidelines on contributions.

## Contact
- Please use [discuss.dgraph.io](https://discuss.dgraph.io) for documentation, questions, feature requests and discussions.
- Please use [Github issue tracker](https://github.com/dgraph-io/dgraph/issues) **ONLY to file bugs.** Any feature request should go to discuss.
- Or, just join [Gitter chat](https://gitter.im/dgraph-io/dgraph?utm_source=share-link&utm_medium=link&utm_campaign=share-link).

## Talks
- [Lightening Talk](http://go-talks.appspot.com/github.com/dgraph-io/dgraph/present/sydney5mins/g.slide#1) on 29th Oct, 2015 at Go meetup, Sydney.

## About
I, [Manish R Jain](https://twitter.com/manishrjain), the author of DGraph, used to work on Google Knowledge Graph.
My experience building large scale, distributed (Web Search and) Graph systems at Google is what inspired me to build this.
