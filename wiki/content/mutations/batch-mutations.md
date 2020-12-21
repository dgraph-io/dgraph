+++
date = "2017-03-20T22:25:17+11:00"
title = "Batch mutations"
weight = 6
[menu.main]
    parent = "mutations"
+++

Each mutation may contain multiple RDF triples. For large data uploads many such mutations can be batched in parallel.  The command `dgraph live` does just this; by default batching 1000 RDF lines into a query, while running 100 such queries in parallel.

`dgraph live` takes as input gzipped N-Quad files (that is triple lists without `{ set {`) and batches mutations for all triples in the input.  The tool has documentation of options.

```sh
dgraph live --help
```
See also [Fast Data Loading]({{< relref "deploy/fast-data-loading.md" >}}).
