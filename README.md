# Dgraph
**Scalable, Distributed, Low Latency, High Throughput Graph Database.**

![logo](https://img.shields.io/badge/status-alpha-red.svg)
[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bhttps%3A%2F%2Fgithub.com%2Fdgraph-io%2Fdgraph.svg?type=shield)](https://app.fossa.io/projects/git%2Bhttps%3A%2F%2Fgithub.com%2Fdgraph-io%2Fdgraph?ref=badge_shield)
[![Wiki](https://img.shields.io/badge/res-wiki-blue.svg)](https://docs.dgraph.io)
[![Build Status](https://travis-ci.org/dgraph-io/dgraph.svg?branch=master)](https://travis-ci.org/dgraph-io/dgraph)
[![Coverage Status](https://coveralls.io/repos/github/dgraph-io/dgraph/badge.svg?branch=master)](https://coveralls.io/github/dgraph-io/dgraph?branch=master)
[![Go Report Card](https://goreportcard.com/badge/github.com/dgraph-io/dgraph)](https://goreportcard.com/report/github.com/dgraph-io/dgraph)
[![Slack Status](http://slack.dgraph.io/badge.svg)](http://slack.dgraph.io)

Dgraph's goal is to provide [Google](https://www.google.com) production level scale and throughput,
with low enough latency to be serving real time user queries, over terabytes of structured data.
Dgraph supports [GraphQL-like query syntax](https://docs.dgraph.io/master/query-language/), and responds in [JSON](http://www.json.org/) and [Protocol Buffers](https://developers.google.com/protocol-buffers/) over [GRPC](http://www.grpc.io/).

## Get Started
**To get started with Dgraph, follow [this 5-step tutorial](https://docs.dgraph.io).**

## Current Status

Dgraph is currently at version 0.7. It has 90% of the features planned for v1.0; and implements RAFT protocol for data replication, high availability and crash recovery. We recommend using it for internal projects at companies. If you plan to use Dgraph for user-facing production environment, [come talk to us](https://discuss.dgraph.io).


## Users
- **Dgraph official documentation is present at [docs.dgraph.io](https://docs.dgraph.io).**
- For feature requests or questions, visit [https://discuss.dgraph.io](https://discuss.dgraph.io).
- Check out [the demo at dgraph.io](http://dgraph.io) and [the visualization at play.dgraph.io](http://play.dgraph.io/).
- Please see [releases tab](https://github.com/dgraph-io/dgraph/releases) to find the latest release and corresponding release notes.
- [See the Roadmap](https://github.com/dgraph-io/dgraph/issues/1) for list of working and planned features.
- Read about the latest updates from Dgraph team [on our blog](https://open.dgraph.io/).

## Developers
- See a list of issues [that we need help with](https://github.com/dgraph-io/dgraph/issues?q=is%3Aissue+is%3Aopen+label%3Ahelp_wanted).
- Please see [contributing to Dgraph](https://wiki.dgraph.io/Contributing_to_Dgraph) for guidelines on contributions.

## Data Loading and Persistence
[![Dgraph data persistence](https://img.youtube.com/vi/dzTEXxF0TGs/0.jpg)](https://www.youtube.com/watch?v=dzTEXxF0TGs)

## Performance

![Loader performance](static/loader.gif)

[See performance page](https://wiki.dgraph.io/Performance) for more details.

## Contact
- Please use [discuss.dgraph.io](https://discuss.dgraph.io) for documentation, questions, feature requests and discussions.
- Please use [Github issue tracker](https://github.com/dgraph-io/dgraph/issues) for filing bugs or feature requests.
- Or, just join [![Slack Status](http://slack.dgraph.io/badge.svg)](http://slack.dgraph.io).
