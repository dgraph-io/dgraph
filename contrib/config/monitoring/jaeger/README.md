# Jaeger

Jaeger is a distributed tracing system that can be integrated with Dgraph.  Included in this section automation to help install Jaeger into your Kubernetes environment.

* [operator](operator/README.md) - use jaeger operator to install `all-in-one` jaeger pod with [badger](https://github.com/dgraph-io/badger) for storage.
* [chart](chart/README.md) - use jaeger helm chart to install distributed jaeger cluster with [ElasticSearch](https://www.elastic.co/) or [Cassandra](https://cassandra.apache.org/) for storage.
