# dgo [![GoDoc](https://godoc.org/github.com/dgraph-io/dgo?status.svg)](https://godoc.org/github.com/dgraph-io/dgo) [![Build Status](https://teamcity.dgraph.io/guestAuth/app/rest/builds/buildType:(id:dgo_integration)/statusIcon.svg)](https://teamcity.dgraph.io/viewLog.html?buildTypeId=dgo_integration&buildId=lastFinished&guest=1)

Official Dgraph Go client which communicates with the server using [gRPC](https://grpc.io/).

Before using this client, we highly recommend that you go through [tour.dgraph.io] and [docs.dgraph.io]
to understand how to run and work with Dgraph.

[docs.dgraph.io]:https://docs.dgraph.io
[tour.dgraph.io]:https://tour.dgraph.io


## Table of contents

- [Install](#install)
- [Using a client](#using-a-client)
  - [Create a client](#create-a-client)
  - [Alter the database](#alter-the-database)
  - [Create a transaction](#create-a-transaction)
  - [Run a mutation](#run-a-mutation)
  - [Run a query](#run-a-query)
  - [Commit a transaction](#commit-a-transaction)
  - [Setting Metadata Headers](#setting-metadata-headers)
- [Development](#development)
  - [Running tests](#running-tests)

## Install

```sh
go get -u -v github.com/dgraph-io/dgo
```

## Using a client

### Create a client

`dgraphClient` object can be initialised by passing it a list of `api.DgraphClient` clients as
variadic arguments. Connecting to multiple Dgraph servers in the same cluster allows for better
distribution of workload.

The following code snippet shows just one connection.

```go
conn, err := grpc.Dial("localhost:9080", grpc.WithInsecure())
if err != nil {
  log.Fatal(err)
}
defer conn.Close()
dgraphClient := dgo.NewDgraphClient(api.NewDgraphClient(conn))
```

### Alter the database

To set the schema, create an instance of `api.Operation` and use the `Alter` endpoint.

```go
op := &api.Operation{
  Schema: `name: string @index(exact) .`,
}
err := dgraphClient.Alter(ctx, op)
// Check error
```

`Operation` contains other fields as well, including `DropAttr` and `DropAll`.
`DropAll` is useful if you wish to discard all the data, and start from a clean
slate, without bringing the instance down. `DropAttr` is used to drop all the data
related to a predicate.

### Create a transaction

To create a transaction, call `dgraphClient.NewTxn()`, which returns a `*dgo.Txn` object. This
operation incurs no network overhead.

It is a good practice to call `txn.Discard()` using a `defer` statement after it is initialized.
Calling `txn.Discard()` after `txn.Commit()` is a no-op and you can call `txn.Discard()` multiple
times with no additional side-effects.

```go
txn := dgraphClient.NewTxn()
defer txn.Discard(ctx)
```

Read-only transactions can be created by calling `c.NewReadOnlyTxn()`. Read-only
transactions are useful to increase read speed because they can circumvent the
usual consensus protocol. Read-only transactions cannot contain mutations and
trying to call `txn.Commit()` will result in an error. Calling `txn.Discard()`
will be a no-op.

### Run a mutation

`txn.Mutate(ctx, mu)` runs a mutation. It takes in a `context.Context` and a
`*api.Mutation` object. You can set the data using JSON or RDF N-Quad format.

To use JSON, use the fields SetJson and DeleteJson, which accept a string
representing the nodes to be added or removed respectively (either as a JSON map
or a list). To use RDF, use the fields SetNquads and DeleteNquads, which accept
a string representing the valid RDF triples (one per line) to added or removed
respectively. This protobuf object also contains the Set and Del fields which
accept a list of RDF triples that have already been parsed into our internal
format. As such, these fields are mainly used internally and users should use
the SetNquads and DeleteNquads instead if they are planning on using RDF.

We define a Person struct to represent a Person and marshal an instance of it to
use with `Mutation` object.

```go
type Person struct {
  Uid  string `json:"uid,omitempty"`
  Name string `json:"name,omitempty"`
}

p := Person{
  Uid:  "_:alice",
  Name: "Alice",
}

pb, err := json.Marshal(p)
if err != nil {
  log.Fatal(err)
}

mu := &api.Mutation{
  SetJson: pb,
}
assigned, err := txn.Mutate(ctx, mu)
if err != nil {
  log.Fatal(err)
}
```

For a more complete example, see
[GoDoc](https://godoc.org/github.com/dgraph-io/dgo#example-package--SetObject).

Sometimes, you only want to commit a mutation, without querying anything
further. In such cases, you can use `mu.CommitNow = true` to indicate that the
mutation must be immediately committed.

### Run a query

You can run a query by calling `txn.Query(ctx, q)`. You will need to pass in a GraphQL+- query string. If
you want to pass an additional map of any variables that you might want to set in the query, call
`txn.QueryWithVars(ctx, q, vars)` with the variables map as third argument.

Let's run the following query with a variable $a:
```go
q := `query all($a: string) {
    all(func: eq(name, $a)) {
      name
    }
  }`

resp, err := txn.QueryWithVars(ctx, q, map[string]string{"$a": "Alice"})
fmt.Println(string(resp.Json))
```

When running a schema query, the schema response is found in the `Schema` field of `api.Response`.

```go
q := `schema(pred: [name]) {
  type
  index
  reverse
  tokenizer
  list
  count
  upsert
  lang
}`

resp, err := txn.Query(ctx, q)
fmt.Println(resp.Schema)
```

### Commit a transaction

A transaction can be committed using the `txn.Commit(ctx)` method. If your transaction
consisted solely of calls to `txn.Query` or `txn.QueryWithVars`, and no calls to
`txn.Mutate`, then calling `txn.Commit` is not necessary.

An error will be returned if other transactions running concurrently modify the same
data that was modified in this transaction. It is up to the user to retry
transactions when they fail.

```go
txn := dgraphClient.NewTxn()
// Perform some queries and mutations.

err := txn.Commit(ctx)
if err == y.ErrAborted {
  // Retry or handle error
}
```

### Setting Metadata Headers
Metadata headers such as authentication tokens can be set through the context of gRPC methods. Below is an example of how to set a header named "auth-token".
```go
// The following piece of code shows how one can set metadata with
// auth-token, to allow Alter operation, if the server requires it.
md := metadata.New(nil)
md.Append("auth-token", "the-auth-token-value")
ctx := metadata.NewOutgoingContext(context.Background(), md)
dg.Alter(ctx, &op)
```

## Development

### Running tests

Make sure you have `dgraph` installed before you run the tests. This script will run the unit and
integration tests.

```sh
go test -v ./...
```
