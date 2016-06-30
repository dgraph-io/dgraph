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
- [Installation: Moved to wiki](https://wiki.dgraph.io/index.php?title=Beginners%27_guide)
- [Data Loading: Moved to Wiki](https://wiki.dgraph.io/index.php?title=Beginners%27_guide#Data_Loading)
- [Queries and Mutations: Moved to wiki](https://wiki.dgraph.io/index.php?title=Beginners%27_guide#Queries_and_Mutations)
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

## Contributing to Dgraph
- See a list of issues [that we need help with](https://github.com/dgraph-io/dgraph/issues?q=is%3Aissue+is%3Aopen+label%3Ahelp_wanted).
- Please see [contributing to Dgraph](https://wiki.dgraph.io/index.php?title=Contributing_to_Dgraph) for guidelines on contributions.
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
