+++
date = "2017-03-20T19:35:35+11:00"
title = "Clients"
+++

## Implementation

Clients can communicate with the server in two different ways:

- **Via [gRPC](http://www.grpc.io/).** Internally this uses [Protocol
  Buffers](https://developers.google.com/protocol-buffers) (the proto file
used by Dgraph is located at
[api.proto](https://github.com/dgraph-io/dgo/blob/master/protos/api.proto)).

- **Via HTTP.** There are various endpoints, each accepting and returning JSON.
  There is a one to one correspondence between the HTTP endpoints and the gRPC
service methods.


It's possible to interface with Dgraph directly via gRPC or HTTP. However, if a
client library exists for you language, this will be an easier option.

{{% notice "tip" %}}
For multi-node setups, predicates are assigned to the group that first sees that
predicate. Dgraph also automatically moves predicate data to different groups in
order to balance predicate distribution. This occurs automatically every 10
minutes. It's possible for clients to aid this process by communicating with all
Dgraph instances. For the Go client, this means passing in one
`*grpc.ClientConn` per Dgraph instance. Mutations will be made in a round robin
fashion, resulting in an initially semi random predicate distribution.
{{% /notice %}}

### Transactions

Dgraph clients perform mutations and queries using transactions. A
transaction bounds a sequence of queries and mutations that are committed by
Dgraph as a single unit: that is, on commit, either all the changes are accepted
by Dgraph or none are. 

A transaction always sees the database state at the moment it began, plus any 
changes it makes --- changes from concurrent transactions aren't visible.

On commit, Dgraph will abort a transaction, rather than committing changes, when
a conflicting, concurrently running transaction has already been committed.  Two
transactions conflict when both transactions: 

- write values to the same scalar predicate of the same node (e.g both
  attempting to set a particular node's `address` predicate); or
- write to a singular `uid` predicate of the same node (changes to `[uid]` predicates can be concurrently written); or
- write a value that conflicts on an index for a predicate with `@upsert` set in the schema (see [upserts]({{< relref "howto/index.md#upserts">}})).

When a transaction is aborted, all its changes are discarded.  Transactions can be manually aborted.

## Go

[![GoDoc](https://godoc.org/github.com/dgraph-io/dgo?status.svg)](https://godoc.org/github.com/dgraph-io/dgo)

The Go client communicates with the server on the gRPC port (default value 9080).

The client can be obtained in the usual way via `go get`:

```sh
go get -u -v github.com/dgraph-io/dgo
```

The full [GoDoc](https://godoc.org/github.com/dgraph-io/dgo) contains
documentation for the client API along with examples showing how to use it.

### Create the client

To create a client, dial a connection to Dgraph's external gRPC port (typically
9080). The following code snippet shows just one connection. You can connect to multiple Dgraph Alphas to distribute the workload evenly.

```go
func newClient() *dgo.Dgraph {
	// Dial a gRPC connection. The address to dial to can be configured when
	// setting up the dgraph cluster.
	d, err := grpc.Dial("localhost:9080", grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}

	return dgo.NewDgraphClient(
		api.NewDgraphClient(d),
	)
}
```

The client can be configured to use gRPC compression:

```go
func newClient() *dgo.Dgraph {
	// Dial a gRPC connection. The address to dial to can be configured when
	// setting up the dgraph cluster.
	dialOpts := append([]grpc.DialOption{},
		grpc.WithInsecure(),
		grpc.WithDefaultCallOptions(grpc.UseCompressor(gzip.Name)))
	d, err := grpc.Dial("localhost:9080", dialOpts...)

	if err != nil {
		log.Fatal(err)
	}

	return dgo.NewDgraphClient(
		api.NewDgraphClient(d),
	)
}

```

### Alter the database

To set the schema, set it on a `api.Operation` object, and pass it down to
the `Alter` method.

```go
func setup(c *dgo.Dgraph) {
	// Install a schema into dgraph. Accounts have a `name` and a `balance`.
	err := c.Alter(context.Background(), &api.Operation{
		Schema: `
			name: string @index(term) .
			balance: int .
		`,
	})
}
```

`api.Operation` contains other fields as well, including drop predicate and drop
all. Drop all is useful if you wish to discard all the data, and start from a
clean slate, without bringing the instance down.

```go
	// Drop all data including schema from the dgraph instance. This is useful
	// for small examples such as this, since it puts dgraph into a clean
	// state.
	err := c.Alter(context.Background(), &api.Operation{DropOp: api.Operation_ALL})
```

The old way to send a drop all operation is still supported but will be eventually
deprecated. It's shown below for reference.

```go
	// Drop all data including schema from the dgraph instance. This is useful
	// for small examples such as this, since it puts dgraph into a clean
	// state.
	err := c.Alter(context.Background(), &api.Operation{DropAll: true})
```

Starting with version 1.1, `api.Operation` also supports a drop data operation.
This operation drops all the data but preserves the schema. This is useful when
the schema is large and needs to be reused, such as in between unit tests.

```go
	// Drop all data including schema from the dgraph instance. This is useful
	// for small examples such as this, since it puts dgraph into a clean
	// state.
	err := c.Alter(context.Background(), &api.Operation{DropOp: api.Operation_DATA})
```

### Create a transaction

Dgraph supports running distributed ACID transactions. To create a
transaction, just call `c.NewTxn()`. This operation incurs no network call.
Typically, you'd also want to call a `defer txn.Discard()` to let it
automatically rollback in case of errors. Calling `Discard` after `Commit` would
be a no-op.

```go
func runTxn(c *dgo.Dgraph) {
	txn := c.NewTxn()
	defer txn.Discard()
	...
}
```

#### Read-Only Transactions

Read-only transactions can be created by calling `c.NewReadOnlyTxn()`. Read-only
transactions are useful to increase read speed because they can circumvent the
usual consensus protocol. Read-only transactions cannot contain mutations and
trying to call `txn.Commit()` will result in an error. Calling `txn.Discard()`
will be a no-op.

Read-only queries can optionally be set as best-effort. Using this flag will ask
the Dgraph Alpha to try to get timestamps from memory on a best-effort basis to
reduce the number of outbound requests to Zero. This may yield improved
latencies in read-bound workloads where linearizable reads are not strictly
needed.

### Run a query

You can run a query by calling `txn.Query`. The response would contain a `JSON`
field, which has the JSON encoded result. You can unmarshal it into Go struct
via `json.Unmarshal`.

```go
	// Query the balance for Alice and Bob.
	const q = `
		{
			all(func: anyofterms(name, "Alice Bob")) {
				uid
				balance
			}
		}
	`
	resp, err := txn.Query(context.Background(), q)
	if err != nil {
		log.Fatal(err)
	}

	// After we get the balances, we have to decode them into structs so that
	// we can manipulate the data.
	var decode struct {
		All []struct {
			Uid     string
			Balance int
		}
	}
	if err := json.Unmarshal(resp.GetJson(), &decode); err != nil {
		log.Fatal(err)
	}
```

### Run a mutation

`txn.Mutate` would run the mutation. It takes in a `api.Mutation` object,
which provides two main ways to set data: JSON and RDF N-Quad. You can choose
whichever way is convenient.

To use JSON, use the fields SetJson and DeleteJson, which accept a string
representing the nodes to be added or removed respectively (either as a JSON map
or a list). To use RDF, use the fields SetNquads and DeleteNquads, which accept
a string representing the valid RDF triples (one per line) to added or removed
respectively. This protobuf object also contains the Set and Del fields which
accept a list of RDF triples that have already been parsed into our internal
format. As such, these fields are mainly used internally and users should use
the SetNquads and DeleteNquads instead if they are planning on using RDF.

We're going to continue using JSON. You could modify the Go structs parsed from
the query, and marshal them back into JSON.

```go
	// Move $5 between the two accounts.
	decode.All[0].Bal += 5
	decode.All[1].Bal -= 5

	out, err := json.Marshal(decode.All)
	if err != nil {
		log.Fatal(err)
	}

	_, err := txn.Mutate(context.Background(), &api.Mutation{SetJson: out})
```

Sometimes, you only want to commit mutation, without querying anything further.
In such cases, you can use a `CommitNow` field in `api.Mutation` to
indicate that the mutation must be immediately committed.

### Commit the transaction

Once all the queries and mutations are done, you can commit the transaction. It
returns an error in case the transaction could not be committed.

```go
	// Finally, we can commit the transactions. An error will be returned if
	// other transactions running concurrently modify the same data that was
	// modified in this transaction. It is up to the library user to retry
	// transactions when they fail.

	err := txn.Commit(context.Background())
```

### Complete Example

This is an example from the [GoDoc](https://godoc.org/github.com/dgraph-io/dgo). It shows how to to create a Node with name Alice, while also creating her relationships with other nodes. Note `loc` predicate is of type `geo` and can be easily marshalled and unmarshalled into a Go struct. More such examples are present as part of the GoDoc.

```go
type School struct {
	Name string `json:"name,omitempty"`
}

type loc struct {
	Type   string    `json:"type,omitempty"`
	Coords []float64 `json:"coordinates,omitempty"`
}

// If omitempty is not set, then edges with empty values (0 for int/float, "" for string, false
// for bool) would be created for values not specified explicitly.

type Person struct {
		Uid      string     `json:"uid,omitempty"`
		Name     string     `json:"name,omitempty"`
		Age      int        `json:"age,omitempty"`
		Dob      *time.Time `json:"dob,omitempty"`
		Married  bool       `json:"married,omitempty"`
		Raw      []byte     `json:"raw_bytes,omitempty"`
		Friends  []Person   `json:"friend,omitempty"`
		Location loc        `json:"loc,omitempty"`
		School   []School   `json:"school,omitempty"`
}

conn, err := grpc.Dial("127.0.0.1:9080", grpc.WithInsecure())
if err != nil {
	log.Fatal("While trying to dial gRPC")
}
defer conn.Close()

dc := api.NewDgraphClient(conn)
dg := dgo.NewDgraphClient(dc)

op := &api.Operation{}
op.Schema = `
	name: string @index(exact) .
	age: int .
	married: bool .
	loc: geo .
	dob: datetime .
`

ctx := context.Background()
err = dg.Alter(ctx, op)
if err != nil {
	log.Fatal(err)
}

dob := time.Date(1980, 01, 01, 23, 0, 0, 0, time.UTC)
// While setting an object if a struct has a Uid then its properties in the graph are updated
// else a new node is created.
// In the example below new nodes for Alice, Bob and Charlie and school are created (since they
// dont have a Uid).
p := Person{
	Name:    "Alice",
	Age:     26,
	Married: true,
	Location: loc{
		Type:   "Point",
		Coords: []float64{1.1, 2},
	},
	Dob: &dob,
	Raw: []byte("raw_bytes"),
	Friends: []Person{{
		Name: "Bob",
		Age:  24,
	}, {
		Name: "Charlie",
		Age:  29,
	}},
	School: []School{{
		Name: "Crown Public School",
	}},
}

mu := &api.Mutation{
	CommitNow: true,
}
pb, err := json.Marshal(p)
if err != nil {
	log.Fatal(err)
}

mu.SetJson = pb
assigned, err := dg.NewTxn().Mutate(ctx, mu)
if err != nil {
	log.Fatal(err)
}

// Assigned uids for nodes which were created would be returned in the resp.AssignedUids map.
variables := map[string]string{"$id": assigned.Uids["blank-0"]}
q := `query Me($id: string){
	me(func: uid($id)) {
		name
		dob
		age
		loc
		raw_bytes
		married
		friend @filter(eq(name, "Bob")){
			name
			age
		}
		school {
			name
		}
	}
}`

resp, err := dg.NewTxn().QueryWithVars(ctx, q, variables)
if err != nil {
	log.Fatal(err)
}

type Root struct {
	Me []Person `json:"me"`
}

var r Root
err = json.Unmarshal(resp.Json, &r)
if err != nil {
	log.Fatal(err)
}
// fmt.Printf("Me: %+v\n", r.Me)
// R.Me would be same as the person that we set above.

fmt.Println(string(resp.Json))
// Output: {"me":[{"name":"Alice","dob":"1980-01-01T23:00:00Z","age":26,"loc":{"type":"Point","coordinates":[1.1,2]},"raw_bytes":"cmF3X2J5dGVz","married":true,"friend":[{"name":"Bob","age":24}],"school":[{"name":"Crown Public School"}]}]}


```


## Java

The official Java client [can be found here](https://github.com/dgraph-io/dgraph4j)
and it fully supports Dgraph v1.0.x. Follow the instructions in the
[README](https://github.com/dgraph-io/dgraph4j#readme)
to get it up and running.

We also have a [DgraphJavaSample] project, which contains an end-to-end
working example of how to use the Java client.

[DgraphJavaSample]:https://github.com/dgraph-io/dgraph4j/tree/master/samples/DgraphJavaSample

## JavaScript

The official JavaScript client [can be found here](https://github.com/dgraph-io/dgraph-js)
and it fully supports Dgraph v1.0.x. Follow the instructions in the
[README](https://github.com/dgraph-io/dgraph-js#readme) to get it up and running.

We also have a [simple example](https://github.com/dgraph-io/dgraph-js/tree/master/examples/simple)
project, which contains an end-to-end working example of how to use the JavaScript client,
for Node.js >= v6.

## Python

The official Python client [can be found here](https://github.com/dgraph-io/pydgraph)
and it fully supports Dgraph v1.0.x and Python versions >= 2.7 and >= 3.5. Follow the
instructions in the [README](https://github.com/dgraph-io/pydgraph#readme) to get it
up and running.

We also have a [simple example](https://github.com/dgraph-io/pydgraph/tree/master/examples/simple)
project, which contains an end-to-end working example of how to use the Python client.

## Unofficial Dgraph Clients

{{% notice "note" %}}
These third-party clients are contributed by the community and are not officially supported by Dgraph.
{{% /notice %}}

### C\#

- https://github.com/AlexandreDaSilva/DgraphNet
- https://github.com/MichaelJCompton/Dgraph-dotnet

### Dart

- https://github.com/katutz/dgraph

### Elixir

- https://github.com/liveforeverx/dlex
- https://github.com/ospaarmann/exdgraph

## Raw HTTP

{{% notice "warning" %}}
Raw HTTP needs more chops to use than our language clients. We wrote this guide to help you build a Dgraph client in a new language.
{{% /notice %}}

It's also possible to interact with Dgraph directly via its HTTP endpoints.
This allows clients to be built for languages that don't have access to a
working gRPC implementation.

In the examples shown here, regular command line tools such as `curl` and
[`jq`](https://stedolan.github.io/jq/) are used. However, the real intention
here is to show other programmers how they could implement a client in their
language on top of the HTTP API. For an example of how to build a client on top
of gRPC, refer to the implementation of the Go client.

Similar to the Go client example, we use a bank account transfer example.

### Create the Client

A client built on top of the HTTP API will need to track three pieces of state
for each transaction.

1. A start timestamp (`start_ts`). This uniquely identifies a transaction,
   and doesn't change over the transaction lifecycle.

2. The set of keys modified by the transaction (`keys`). This aids in
   transaction conflict detection.

     Every mutation would send back a new set of keys. The client must merge them
     with the existing set. Optionally, a client can de-dup these keys while
     merging.

3. The set of predicates modified by the transaction (`preds`). This aids in
   predicate move detection.

     Every mutation would send back a new set of preds. The client must merge them
     with the existing set. Optionally, a client can de-dup these keys while
     merging.

### Alter the database

The `/alter` endpoint is used to create or change the schema. Here, the
predicate `name` is the name of an account. It's indexed so that we can look up
accounts based on their name.

```sh
$ curl -X POST localhost:8080/alter -d 'name: string @index(term) .'
```

If all goes well, the response should be `{"code":"Success","message":"Done"}`.

Other operations can be performed via the `/alter` endpoint as well. A specific
predicate or the entire database can be dropped.

E.g. to drop the predicate `name`:
```sh
$ curl -X POST localhost:8080/alter -d '{"drop_attr": "name"}'
```
To drop all data and schema:
```sh
$ curl -X POST localhost:8080/alter -d '{"drop_all": true}'
```

### Start a transaction

Assume some initial accounts with balances have been populated. We now want to
transfer money from one account to the other. This is done in four steps:

1. Create a new transaction.

1. Inside the transaction, run a query to determine the current balances.

2. Perform a mutation to update the balances.

3. Commit the transaction.

Starting a transaction doesn't require any interaction with Dgraph itself.
Some state needs to be set up for the transaction to use. The `start_ts`
can initially be set to 0. `keys` can start as an empty set.

**For both query and mutation if the `start_ts` is provided as a path parameter,
then the operation is performed as part of the ongoing transaction. Otherwise, a
new transaction is initiated.**

### Run a query

To query the database, the `/query` endpoint is used. Remember to set the `Content-Type` header
to `application/graphqlpm` in order to ensure that the body of the request is correctly parsed.

To get the balances for both accounts:

```sh
$ curl -H "Content-Type: application/graphqlpm" -X POST localhost:8080/query -d $'
{
  balances(func: anyofterms(name, "Alice Bob")) {
    uid
    name
    balance
  }
}' | jq

```

The result should look like this:

```json
{
  "data": {
    "balances": [
      {
        "uid": "0x1",
        "name": "Alice",
        "balance": "100"
      },
      {
        "uid": "0x2",
        "name": "Bob",
        "balance": "70"
      }
    ]
  },
  "extensions": {
    "server_latency": {
      "parsing_ns": 70494,
      "processing_ns": 697140,
      "encoding_ns": 1560151
    },
    "txn": {
      "start_ts": 4,
    }
  }
}
```

Notice that along with the query result under the `data` field is additional
data in the `extensions -> txn` field. This data will have to be tracked by the
client.

For queries, there is a `start_ts` in the response. This `start_ts` will need to
be used in all subsequent interactions with Dgraph for this transaction, and so
should become part of the transaction state.

### Run a Mutation

Now that we have the current balances, we need to send a mutation to Dgraph
with the updated balances. If Bob transfers $10 to Alice, then the RDFs to send
are:

```
<0x1> <balance> "110" .
<0x2> <balance> "60" .
```

Note that we have to refer to the Alice and Bob nodes by UID in the RDF format.

We now send the mutations via the `/mutate` endpoint. We need to provide our
transaction start timestamp as a path parameter, so that Dgraph knows which
transaction the mutation should be part of. We also need to set `Content-Type`
header to `application/rdf` in order to specify that mutation is written in
rdf format.

```sh
$ curl -H "Content-Type: application/rdf" -X POST localhost:8080/mutate?startTs=4 -d $'
{
  set {
    <0x1> <balance> "110" .
    <0x2> <balance> "60" .
  }
}
' | jq
```

The result:

```json
{
  "data": {
    "code": "Success",
    "message": "Done",
    "uids": {}
  },
  "extensions": {
    "server_latency": {
      "parsing_ns": 17000,
      "processing_ns": 4722207
    },
    "txn": {
      "start_ts": 4,
      "keys": [
        "i4elpex2rwx3",
        "nkvfdz3ltmvv"
      ]
      "preds": [
        "1-balance",
        "1-_predicate_"
      ]
    }
  }
}
```

We get some `keys`. These should be added to the set of `keys` stored in the
transaction state. We also get some `preds`, which should be added to the set of
`preds` stored in the transaction state.

### Committing the transaction

{{% notice "note" %}}
It's possible to commit immediately after a mutation is made (without requiring
to use the `/commit` endpoint as explained in this section). To do this, add
the parameter `commitNow` in the URL `/mutate?commitNow=true`.
{{% /notice %}}

Finally, we can commit the transaction using the `/commit` endpoint. We need the
`start_ts` we've been using for the transaction along with the `keys` and the
`preds`. If we had performed multiple mutations in the transaction instead of
just one, then the keys and preds provided during the commit would be the union
of all keys and preds returned in the responses from the `/mutate` endpoint.

The `preds` field is used to abort the transaction in cases where some of the
predicates are moved. This field is not required and the `/commit` endpoint also
accepts the old format, which was a single array of keys.

```sh
$ curl -X POST localhost:8080/commit?startTs=4 -d $'
{
    "keys": [
		"i4elpex2rwx3",
		"nkvfdz3ltmvv"
	],
	"preds": [
		"1-predicate",
		"1-name"
	]
}' | jq
```

```json
{
  "data": {
    "code": "Success",
    "message": "Done"
  },
  "extensions": {
    "txn": {
      "start_ts": 4,
      "commit_ts": 5
    }
  }
}
```
The transaction is now complete.

If another client were to perform another transaction concurrently affecting
the same keys, then it's possible that the transaction would *not* be
successful.  This is indicated in the response when the commit is attempted.

```json
{
  "errors": [
    {
      "code": "Error",
      "message": "Transaction has been aborted. Please retry."
    }
  ]
}
```

In this case, it should be up to the user of the client to decide if they wish
to retry the transaction.

### Compression via HTTP

Dgraph supports gzip-compressed requests to and from Dgraph Alphas for `/query`, `/mutate`, and `/alter`.

Compressed requests: To send compressed requests, set the HTTP request header
`Content-Encoding: gzip` along with the gzip-compressed payload.

Compressed responses: To receive gzipped responses, set the HTTP request header
`Accept-Encoding: gzip` and Alpha will return gzipped responses.

Example of a compressed request via curl:

```sh
$ curl -X POST \
  -H 'Content-Encoding: gzip' \
  -H "Content-Type: application/rdf" \
  localhost:8080/mutate?commitNow=true --data-binary @mutation.gz
```

Example of a compressed request via curl:

```sh
$ curl -X POST \
  -H 'Accept-Encoding: gzip' \
  -H "Content-Type: application/graphqlpm" \
  localhost:8080/query -d $'schema {}' | gzip --decompress
```

Example of a compressed request and response via curl:

```sh
$ zcat query.gz # query.gz is gzipped compressed
{
  all(func: anyofterms(name, "Alice Bob")) {
    uid
    balance
  }
}
```

```sh
$ curl -X POST \
  -H 'Content-Encoding: gzip' \
  -H 'Accept-Encoding: gzip' \
  -H "Content-Type: application/graphqlpm" \
  localhost:8080/query --data-binary @query.gz | gzip --decompress
```

{{% notice "note" %}}
Curl has a `--compressed` option that automatically requests for a compressed response (`Accept-Encoding` header) and decompresses the compressed response.

```sh
$ curl -X POST --compressed -H "Content-Type: application/graphqlpm" localhost:8080/query -d $'schema {}'
```
{{% /notice %}}
