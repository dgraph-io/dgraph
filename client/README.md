# Go client

[![GoDoc](https://godoc.org/github.com/dgraph-io/dgraph/client?status.svg)](https://godoc.org/github.com/dgraph-io/dgraph/client)

This package provides helper function for interacting with the Dgraph server.
You can use it to run mutations and queries. You can also use BatchMutation
to upload data concurrently. It communicates with the server using gRPC.
