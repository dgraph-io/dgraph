![](/logo.png)

**Fast, Transactional, Distributed Graph Database.**

[![Wiki](https://img.shields.io/badge/res-wiki-blue.svg)](https://docs.dgraph.io)
[![Build Status](https://teamcity.dgraph.io/guestAuth/app/rest/builds/buildType:(id:Dgraph_Ci)/statusIcon.svg)](https://teamcity.dgraph.io/viewLog.html?buildTypeId=Dgraph_Ci&buildId=lastFinished&guest=1)
[![Coverage Status](https://coveralls.io/repos/github/dgraph-io/dgraph/badge.svg?branch=master)](https://coveralls.io/github/dgraph-io/dgraph?branch=master)
[![Go Report Card](https://goreportcard.com/badge/github.com/dgraph-io/dgraph)](https://goreportcard.com/report/github.com/dgraph-io/dgraph)
[![Slack Status](http://slack.dgraph.io/badge.svg)](http://slack.dgraph.io)


Dgraph is a horizontally scalable and distributed graph database, providing ACID transactions, consistent replication and linearizable reads. It's built from ground up to perform for
a rich set of queries. Being a native graph database, it tightly controls how the
data is arranged on disk to optimize for query performance and throughput,
reducing disk seeks and network calls in a cluster.


Dgraph's goal is to provide [Google](https://www.google.com) production level scale and throughput,
with low enough latency to be serving real time user queries, over terabytes of structured data.
Dgraph supports [GraphQL-like query syntax](https://docs.dgraph.io/master/query-language/), and responds in [JSON](http://www.json.org/) and [Protocol Buffers](https://developers.google.com/protocol-buffers/) over [GRPC](http://www.grpc.io/) and HTTP.

## Status

Dgraph is [at version 1.0.0][rel] and is production ready.

[rel]: https://github.com/dgraph-io/dgraph/releases

## Quick Install

The quickest way to install Dgraph is to run this command on Linux or Mac.

```bash
curl https://get.dgraph.io -sSf | bash
```

## Get Started
**To get started with Dgraph, follow:**

- Installation to queries in 3 steps via [docs.dgraph.io](https://docs.dgraph.io/get-started/).
- A longer interactive tutorial via [tour.dgraph.io](https://tour.dgraph.io).
- Tutorial and
presentation videos on [YouTube channel](https://www.youtube.com/channel/UCghE41LR8nkKFlR3IFTRO4w/featured).

## Is Dgraph the right choice for me?

- Do you have more than 10 SQL tables, connected to each other via foreign ids?
- Do you have sparse data, which doesn't correctly fit into SQL tables?
- Do you want a simple and flexible schema, which is readable and maintainable
  over time?
- Do you care about speed and performance at scale?

If the answers to the above are YES, then Dgraph would be a great fit for your
application. Dgraph provides NoSQL like scalability while providing SQL like
transactions and ability to select, filter and aggregate data points. It
combines that with distributed joins, traversals and graph operations, which
makes it easy to build applications with it.

## Dgraph compared to other graph DBs

| Features | Dgraph | Neo4j | Janus Graph |
| -------- | ------ | ----- | ----------- |
| Architecture | Sharded and Distributed | Single server (+ replicas in enterprise) | Layer on top of other distributed DBs |
| Replication | Consistent | None in community edition (only available in enterprise) | Via underlying DB |
| Data movement for shard rebalancing | Automatic | Not applicable (all data lies on each server) | Via underlying DB |
| Language | GraphQL inspired | Cypher, Gremlin | Gremlin |
| Protocols | Grpc / HTTP + JSON / RDF | Bolt + Cypher | Websocket / HTTP |
| Transactions | Distributed ACID transactions | Single server ACID transactions | Not typically ACID
| Full Text Search | Native support | Native support | Via External Indexing System |
| Regular Expressions | Native support | Native support | Via External Indexing System |
| Geo Search | Native support | External support only | Via External Indexing System |
| License | Apache 2.0 + Commons Clause | GPL v3 | Apache 2.0 |

## Users
- **Dgraph official documentation is present at [docs.dgraph.io](https://docs.dgraph.io).**
- For feature requests or questions, visit
  [https://discuss.dgraph.io](https://discuss.dgraph.io).
- Check out [the demo at dgraph.io](http://dgraph.io) and [the visualization at
  play.dgraph.io](http://play.dgraph.io/).
- Please see [releases tab](https://github.com/dgraph-io/dgraph/releases) to
  find the latest release and corresponding release notes.
- [See the Roadmap](https://github.com/dgraph-io/dgraph/issues/1) for list of
  working and planned features.
- Read about the latest updates from Dgraph team [on our
  blog](https://open.dgraph.io/).
- Watch tech talks on our [YouTube
  channel](https://www.youtube.com/channel/UCghE41LR8nkKFlR3IFTRO4w/featured).

## Developers
- See a list of issues [that we need help with](https://github.com/dgraph-io/dgraph/issues?q=is%3Aissue+is%3Aopen+label%3Ahelp_wanted).
- Please see [contributing to Dgraph](https://docs.dgraph.io/contribute/) for guidelines on contributions.


## Contact
- Please use [discuss.dgraph.io](https://discuss.dgraph.io) for documentation, questions, feature requests and discussions.
- Please use [Github issue tracker](https://github.com/dgraph-io/dgraph/issues) for filing bugs or feature requests.
- Join [![Slack Status](http://slack.dgraph.io/badge.svg)](http://slack.dgraph.io).
- Follow us on Twitter [@dgraphlabs](https://twitter.com/dgraphlabs).

