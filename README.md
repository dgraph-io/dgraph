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

## Users
- **Dgraph official documentation is present at [wiki.dgraph.io](https://wiki.dgraph.io).**
- Note that we use the Github Issue Tracker for bug reports only.
- For feature requests or questions, visit [https://discuss.dgraph.io](https://discuss.dgraph.io).
- Check out [the demo at dgraph.io](http://dgraph.io).
- There's an instance of Dgraph running at http://dgraph.xyz, that you can query without installing Dgraph.
This instance contains 21M facts from [Freebase Film Data](http://www.freebase.com/film).
`curl dgraph.xyz/query -XPOST -d '{}'`

## Developers
- We are using [Git Flow](https://github.com/nvie/gitflow) branching model. So, please send out your pull requests against `develop` branch.
- See a list of issues [that we need help with](https://github.com/dgraph-io/dgraph/issues?q=is%3Aissue+is%3Aopen+label%3Ahelp_wanted).
- Please see [contributing to Dgraph](https://wiki.dgraph.io/Contributing_to_Dgraph) for guidelines on contributions.
- *Alpha Program*: If you want to contribute to Dgraph on a continuous basis and need some Bitcoins to pay for healthy food, talk to us.

## Current Status

**Latest Release: v0.4.1**

Master branch contains the latest stable release.

`Jul 2016 - v0.4`
This release allows for better debugging via net/tracer and provides binaries for Linux and Mac.
[Follow our Trello board](https://trello.com/b/TRVKizWt) for progress.

`May 2016 - v0.3`
This release contains more efficient binary protocol client and ability to query `first:N` results.
[See our Trello board](https://trello.com/b/PF4nZ1vH) for more information.

`Mar 2016 - v0.2`
This is the first truly distributed version of Dgraph.
Please see the [release notes here](https://discuss.dgraph.io/t/dgraph-v0-2-release/17).

`MVP launch - Dec 2015 - v0.1`
This is a minimum viable product, alpha release of Dgraph.
This version is not distributed and support for GraphQL is partial.
[See the Roadmap](https://github.com/dgraph-io/dgraph/issues/1) for list of working and planned features.


## Contact
- Please use [discuss.dgraph.io](https://discuss.dgraph.io) for documentation, questions, feature requests and discussions.
- Please use [Github issue tracker](https://github.com/dgraph-io/dgraph/issues) **ONLY to file bugs.** Any feature request should go to discuss.
- Or, just join [![Slack Status](http://slack.dgraph.io/badge.svg)](http://slack.dgraph.io).
