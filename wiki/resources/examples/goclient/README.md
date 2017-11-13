# go client examples

Examples of using the go client with Dgraph.  Built for Dgraph version 0.8.

* crawlerRDF queries Dgraph, unmarshals results into custom structs and writes out an RDF file that can be loaded with dgraph-live-loader.
* crawlermutation multithreaded crawler that shows how to use client with go routines.  Concurrent crawlers query a source Dgraph, unmarshal into custom structs and write to a target Dgraph.  Shows how to issue mutations using requests in client.
* movielensbatch reads from CSV and uses the client batch interface to write to Dgraph.

Check out the [blog post](https://open.dgraph.io/post/client0.8.0) about these examples.