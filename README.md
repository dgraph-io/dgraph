# Dgraph
**Scalable, Distributed, Low Latency, High Throughput Graph Database.**

![logo](https://img.shields.io/badge/status-alpha-red.svg)
[![Wiki](https://img.shields.io/badge/res-wiki-blue.svg)](http://wiki.dgraph.io)
[![Build Status](https://travis-ci.org/dgraph-io/dgraph.svg?branch=master)](https://travis-ci.org/dgraph-io/dgraph)
[![Coverage Status](https://coveralls.io/repos/github/dgraph-io/dgraph/badge.svg?branch=develop)](https://coveralls.io/github/dgraph-io/dgraph?branch=develop)
[![Slack Status](http://slack.dgraph.io/badge.svg)](http://slack.dgraph.io)


Dgraph's goal is to provide [Google](https://www.google.com) production level scale and throughput,
with low enough latency to be serving real time user queries, over terabytes of structured data.
Dgraph supports [GraphQL](http://graphql.org/) as query language, and responds in [JSON](http://www.json.org/).

---

**Note that we use the Github Issue Tracker for bug reports only.**
**For feature requests or questions, visit [https://discuss.dgraph.io](https://discuss.dgraph.io).**

---

**We are using [Git Flow](https://github.com/nvie/gitflow) branching model. So, please send out your pull requests against `develop` branch.**

---

The README is divided into these sections:
- [Current Status](#current-status)
- [Quick Testing](#quick-testing)
- [Installation](#installation)
- [Usage](#usage)
- [Queries and Mutations](#queries-and-mutations)
- [Contact](#contact)

## Current Status

*Check out [the demo at dgraph.io](http://dgraph.io).*

`Upcoming - v0.4`
[Follow our Trello board](https://trello.com/b/TRVKizWt) for progress.
Got questions or issues? Talk to us [via discuss](https://discuss.dgraph.io).

`May 2016 - Tag v0.3`
This release contains more efficient binary protocol client and ability to query `first:N` results.
Please see [Release notes](https://github.com/dgraph-io/dgraph/releases/tag/v0.3)
and [Trello board](https://trello.com/b/PF4nZ1vH) for more information.

`Mar 2016 - Branch v0.2`
This is the first truly distributed version of Dgraph.
Please see the [release notes here](https://discuss.dgraph.io/t/dgraph-v0-2-release/17).

`MVP launch - Dec 2015 - Branch v0.1`
This is a minimum viable product, alpha release of Dgraph. **It's not meant for production use.**
This version is not distributed and support for GraphQL is partial.
[See the Roadmap](https://github.com/dgraph-io/dgraph/issues/1) for list of working and planned features.

Your feedback is welcome. Feel free to [file an issue](https://github.com/dgraph-io/dgraph/issues)
when you encounter bugs and to direct the development of Dgraph.

There's an instance of Dgraph running at http://dgraph.xyz, that you can query without installing Dgraph.
This instance contains 21M facts from [Freebase Film Data](http://www.freebase.com/film).
See [Queries and Mutations below](#queries-and-mutations) for sample queries.
`curl dgraph.xyz/query -XPOST -d '{}'`


## Quick Testing

### Single instance via Docker
There's a docker image that you can readily use for playing with Dgraph.
```
$ docker pull dgraph/dgraph:latest
# Setting a `somedir` volume on the host will persist your data.
$ docker run -t -i -v /somedir:/dgraph -p 80:8080 dgraph/dgraph:latest
```

Now that you're within the Docker instance, you can start the server.
```
$ mkdir /dgraph/m # Ensure mutations directory exists.
$ dgraph --mutations /dgraph/m --postings /dgraph/p --uids /dgraph/u
```
There are some more options that you can change. Run `dgraph --help` to look at them.

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
For quick testing, you can bring up 3 different processes of Dgraph. You can of course, also set this up across multiple servers.
```
go build . && ./dgraph --instanceIdx 0 --mutations $DIR/m0 --port 8080 --postings $DIR/p0 --workers ":12345,:12346,:12347" --uids $DIR/uasync.final --workerport ":12345" &
go build . && ./dgraph --instanceIdx 1 --mutations $DIR/m1 --port 8082 --postings $DIR/p1 --workers ":12345,:12346,:12347" --workerport ":12346" &
go build . && ./dgraph --instanceIdx 2 --mutations $DIR/m2 --port 8084 --postings $DIR/p2 --workers ":12345,:12346,:12347" --workerport ":12347" &
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
Dgraph depends on [RocksDB](https://github.com/facebook/rocksdb) for storing posting lists.

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

### Install Dgraph
Now get [Dgraph](https://github.com/dgraph-io/dgraph) code. Dgraph uses `govendor` to fix dependency versions. Version information for these dependencies is included in the `github.com/dgraph-io/dgraph/vendor` directory under the `vendor.json` file.  

```
go get -u github.com/kardianos/govendor
# cd to dgraph codebase root directory e.g. $GOPATH/src/github.com/dgraph-io/dgraph
govendor sync

# Optional
go test github.com/dgraph-io/dgraph/...

```
See [govendor](https://github.com/kardianos/govendor) for more information.

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
$ cd $GOPATH/src/github.com/dgraph-io/dgraph/dgraph/dgraphassigner

# Run instance 0.
$ go build . && ./dgraphassigner --numInstances 2 --instanceIdx 0 --rdfgzips $BENCHMARK_REPO/data/rdf-films.gz,$BENCHMARK_REPO/data/names.gz --uids ~/dgraph/uids/u0

# And either later, or on another server, run instance 1.
$ go build . && ./dgraphassigner --numInstances 2 --instanceIdx 1 --rdfgzips $BENCHMARK_REPO/data/rdf-films.gz,$BENCHMARK_REPO/data/names.gz --uids ~/dgraph/uids/u1
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
$ cd $GOPATH/src/github.com/dgraph-io/dgraph/dgraph/dgraphloader
$ go build . && ./dgraphloader --numInstances 3 --instanceIdx 0 --rdfgzips $BENCHMARK_REPO/data/names.gz,$BENCHMARK_REPO/data/rdf-films.gz --uids ~/dgraph/uasync.final --postings ~/dgraph/p0
$ go build . && ./dgraphloader --numInstances 3 --instanceIdx 1 --rdfgzips $BENCHMARK_REPO/data/names.gz,$BENCHMARK_REPO/data/rdf-films.gz --uids ~/dgraph/uasync.final --postings ~/dgraph/p1
$ go build . && ./dgraphloader --numInstances 3 --instanceIdx 2 --rdfgzips $BENCHMARK_REPO/data/names.gz,$BENCHMARK_REPO/data/rdf-films.gz --uids ~/dgraph/uasync.final --postings ~/dgraph/p2
```
You can run these over multiple machines, or just one after another.

### Server
Now that the data is loaded, you can run the Dgraph servers. To serve the 3 shards above, you can follow the [same steps as here](#multiple-distributed-instances).
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

## Queries and Mutations
You can see a list of [sample queries here](https://discuss.dgraph.io/t/list-of-test-queries/22).
Dgraph also supports mutations via GraphQL syntax.
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
Dgraph would assume that any data in `<>` is an external id (XID),
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


## Contributing to Dgraph
- See a list of issues [that we need help with](https://github.com/dgraph-io/dgraph/issues?q=is%3Aissue+is%3Aopen+label%3Ahelp_wanted).
- Please see [contributing to Dgraph](https://discuss.dgraph.io/t/contributing-to-dgraph/20) for guidelines on contributions.
- *Alpha Program*: If you want to contribute to Dgraph on a continuous basis and need some Bitcoins to pay for healthy food, talk to us.

## Contact
- Please use [discuss.dgraph.io](https://discuss.dgraph.io) for documentation, questions, feature requests and discussions.
- Please use [Github issue tracker](https://github.com/dgraph-io/dgraph/issues) **ONLY to file bugs.** Any feature request should go to discuss.
- Or, just join [![Slack Status](http://slack.dgraph.io/badge.svg)](http://slack.dgraph.io).

## Talks
- [Lightening Talk](http://go-talks.appspot.com/github.com/dgraph-io/dgraph/present/sydney5mins/g.slide#1) on 29th Oct, 2015 at Go meetup, Sydney.

## About
I, [Manish R Jain](https://twitter.com/manishrjain), the author of Dgraph, used to work on Google Knowledge Graph.
My experience building large scale, distributed (Web Search and) Graph systems at Google is what inspired me to build this.
