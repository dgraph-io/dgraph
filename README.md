<picture>
      <source 
        srcset="/logo-dark.png"
        media="(prefers-color-scheme: dark)"
      />
      <source
        srcset="/logo.png"
        media="(prefers-color-scheme: light), (prefers-color-scheme: no-preference)"
      />
      <img alt="Dgraph Logo" src="/logo.png">
</picture>

**The Only Native GraphQL Database With A Graph Backend.**

[![Wiki](https://img.shields.io/badge/res-wiki-blue.svg)](https://dgraph.io/docs/)
[![ci-dgraph-tests](https://github.com/dgraph-io/dgraph/actions/workflows/ci-dgraph-tests.yml/badge.svg)](https://github.com/dgraph-io/dgraph/actions/workflows/ci-dgraph-tests.yml)
[![ci-dgraph-load-tests](https://github.com/dgraph-io/dgraph/actions/workflows/ci-dgraph-load-tests.yml/badge.svg)](https://github.com/dgraph-io/dgraph/actions/workflows/ci-dgraph-load-tests.yml)
[![ci-golang-lint](https://github.com/dgraph-io/dgraph/actions/workflows/ci-golang-lint.yml/badge.svg)](https://github.com/dgraph-io/dgraph/actions/workflows/ci-golang-lint.yml)
[![ci-aqua-security-trivy-tests](https://github.com/dgraph-io/dgraph/actions/workflows/ci-aqua-security-trivy-tests.yml/badge.svg)](https://github.com/dgraph-io/dgraph/actions/workflows/ci-aqua-security-trivy-tests.yml)
[![Coverage Status](https://coveralls.io/repos/github/dgraph-io/dgraph/badge.svg?branch=main)](https://coveralls.io/github/dgraph-io/dgraph?branch=main)
[![Go Report Card](https://goreportcard.com/badge/github.com/dgraph-io/dgraph)](https://goreportcard.com/report/github.com/dgraph-io/dgraph)
[![TODOs](https://badgen.net/https/api.tickgit.com/badgen/github.com/dgraph-io/dgraph/main)](https://www.tickgit.com/browse?repo=github.com/dgraph-io/dgraph&branch=main)

Dgraph is a horizontally scalable and distributed GraphQL database with a graph backend. It provides ACID transactions, consistent replication, and linearizable reads. It's built from the ground up to perform for
a rich set of queries. Being a native GraphQL database, it tightly controls how the
data is arranged on disk to optimize for query performance and throughput,
reducing disk seeks and network calls in a cluster.


Dgraph's goal is to provide [Google](https://www.google.com) production level scale and throughput,
with low enough latency to serve real-time user queries over terabytes of structured data.
Dgraph supports [GraphQL query syntax](https://dgraph.io/docs/master/query-language/), and responds in [JSON](http://www.json.org/) and [Protocol Buffers](https://developers.google.com/protocol-buffers/) over [GRPC](http://www.grpc.io/) and HTTP. Dgraph is written using the Go Programming Language.

## Status

Dgraph is at [version v22.0.2][rel] and is production-ready. Apart from the vast open source community, it is being used in
production at multiple Fortune 500 companies, and by
[Intuit Katlas](https://github.com/intuit/katlas) and [VMware Purser](https://github.com/vmware/purser). A hosted version of Dgraph is available at [https://cloud.dgraph.io](https://cloud.dgraph.io).

[rel]: https://github.com/dgraph-io/dgraph/releases/tag/v22.0.0

## Supported Platforms

Dgraph officially supports the Linux/amd64 architecture. Support for Linux/arm64 is in development. In order to take advantage of memory performance gains and other architecture-specific advancements in Linux, we dropped official support Mac and Windows in 2021, see [this blog post](https://discuss.dgraph.io/t/dropping-support-for-windows-and-mac/12913) for more information. You can still build and use Dgraph on other platforms (for live or bulk loading for instance), but support for platforms other than Linux/amd64 is not available.

Running Dgraph in a Docker environment is the recommended testing and deployment method.

## Install with Docker

If you're using Docker, you can use the [official Dgraph image](https://hub.docker.com/r/dgraph/dgraph/).

```bash
docker pull dgraph/dgraph:latest
```

For more information on a variety Docker deployment methods including Docker Compose and Kubernetes, see the [docs](https://dgraph.io/docs/deploy/single-host-setup/#run-using-docker).

## Run a Quick Standalone Cluster

```
docker run -it -p 8080:8080 -p 9080:9080 -v ~/dgraph:/dgraph dgraph/standalone:latest
```

## Install from Source

If you want to install from source, install Go 1.13+ or later and the following dependencies:

#### Ubuntu

```bash
sudo apt-get update
sudo apt-get install build-essential
```

#### macOS

As a prerequisite, first install [XCode](https://apps.apple.com/us/app/xcode/id497799835?mt=12) (or the [XCode Command-line Tools](https://developer.apple.com/downloads/)) and [Homebrew](https://brew.sh/).

Next, install the required dependencies:

```bash
brew update
brew install jemalloc go
```

### Build and Install

Then clone the Dgraph repository and use `make install` to install the Dgraph binary in the directory named by the GOBIN environment variable, which defaults to $GOPATH/bin or $HOME/go/bin if the GOPATH environment variable is not set. 


```bash
git clone https://github.com/dgraph-io/dgraph.git
cd dgraph
make install
```

## Get Started
**To get started with Dgraph, follow:**

- Installation to queries in 3 steps via [dgraph.io/docs/](https://dgraph.io/docs/get-started/).
- A longer interactive tutorial via [dgraph.io/tour/](https://dgraph.io/tour/).
- Tutorial and
presentation videos on [YouTube channel](https://www.youtube.com/channel/UCghE41LR8nkKFlR3IFTRO4w/featured).

## Is Dgraph the right choice for me?

- Do you have more than 10 SQL tables connected via foreign keys?
- Do you have sparse data, which doesn't elegantly fit into SQL tables?
- Do you want a simple and flexible schema, which is readable and maintainable
  over time?
- Do you care about speed and performance at scale?

If the answers to the above are YES, then Dgraph would be a great fit for your
application. Dgraph provides NoSQL like scalability while providing SQL like
transactions and the ability to select, filter, and aggregate data points. It
combines that with distributed joins, traversals, and graph operations, which
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
| Full-Text Search | Native support | Native support | Via External Indexing System |
| Regular Expressions | Native support | Native support | Via External Indexing System |
| Geo Search | Native support | External support only | Via External Indexing System |
| License | Apache 2.0 | GPL v3 | Apache 2.0 |

## Users
- **Dgraph official documentation is present at [dgraph.io/docs/](https://dgraph.io/docs/).**
- For feature requests or questions, visit
  [https://discuss.dgraph.io](https://discuss.dgraph.io).
- Check out [the demo at dgraph.io](http://dgraph.io) and [the visualization at
  play.dgraph.io](http://play.dgraph.io/).
- Please see [releases tab](https://github.com/dgraph-io/dgraph/releases) to
  find the latest release and corresponding release notes.
- [See the Roadmap](https://discuss.dgraph.io/t/product-roadmap-2020/8479) for a list of
  working and planned features.
- Read about the latest updates from the Dgraph team [on our
  blog](https://open.dgraph.io/).
- Watch tech talks on our [YouTube
  channel](https://www.youtube.com/channel/UCghE41LR8nkKFlR3IFTRO4w/featured).

## Developers
- See a list of issues [that we need help with](https://github.com/dgraph-io/dgraph/issues?q=is%3Aissue+is%3Aopen+label%3A%22help+wanted%22).
- Please see [Contributing to Dgraph](https://github.com/dgraph-io/dgraph/blob/master/CONTRIBUTING.md) for guidelines on contributions.

## Client Libraries
The Dgraph team maintains several [officially supported client libraries](https://dgraph.io/docs/clients/). There are also libraries contributed by the community [unofficial client libraries](https://dgraph.io/docs/clients#unofficial-dgraph-clients).

## Contact
- Please use [discuss.dgraph.io](https://discuss.dgraph.io) for documentation, questions, feature requests and discussions.
- Please use [discuss.dgraph.io](https://discuss.dgraph.io/c/issues/dgraph/38) for filing bugs or feature requests.
- Follow us on Twitter [@dgraphlabs](https://twitter.com/dgraphlabs).
