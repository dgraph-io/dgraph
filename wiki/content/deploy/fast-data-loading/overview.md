+++
date = "2017-03-20T22:25:17+11:00"
title = "Overview"
weight = 1
[menu.main]
    parent = "fast-data-loading"
    identifier = "data-loading-overview"
+++

There are two different tools that can be used for fast data loading:

- `dgraph live` runs the [Dgraph Live Loader]({{< relref "live-loader.md" >}})
- `dgraph bulk` runs the [Dgraph Bulk Loader]({{< relref "bulk-loader.md" >}})

{{% notice "note" %}} Both tools only accept [RDF N-Quad/Triple
data](https://www.w3.org/TR/n-quads/) or JSON in plain or gzipped format. Data
in other formats must be converted.{{% /notice %}}

## Live Loader

[Dgraph Live Loader]({{< relref "live-loader.md" >}}) (run with `dgraph live`) is a small helper program which reads RDF N-Quads from a gzipped file, batches them up, creates mutations (using the go client) and shoots off to Dgraph.

## Bulk Loader

[Dgraph Bulk Loader]({{< relref "bulk-loader.md" >}}) serves a similar purpose to the Dgraph Live Loader, but can
only be used to load data into a new cluster. It cannot be run on an existing
Dgraph cluster. Dgraph Bulk Loader is **considerably faster** than the Dgraph
Live Loader and is the recommended way to perform the initial import of large
datasets into Dgraph.
