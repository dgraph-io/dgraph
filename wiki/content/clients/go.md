+++
date = "2017-03-20T22:25:17+11:00"
title = "Go"
weight = 2
[menu.main]
    parent = "clients"
+++

[![GoDoc](https://godoc.org/github.com/dgraph-io/dgo?status.svg)](https://godoc.org/github.com/dgraph-io/dgo)

The Go client communicates with the server on the gRPC port (default value 9080).

The client can be obtained in the usual way via `go get`:

```sh
 Requires at least Go 1.11
export GO111MODULE=on
go get -u -v github.com/dgraph-io/dgo/v200
```

The full [GoDoc](https://godoc.org/github.com/dgraph-io/dgo) contains
documentation for the client API along with examples showing how to use it.

More details on the supported versions can be found at [this link](https://github.com/dgraph-io/dgo#supported-versions).

## Create the client

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

## Alter the database

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

## Create a transaction

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

### Read-Only Transactions

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

## Run a query

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

## Query for RDF response.

You can get query result as a RDF response by calling `txn.QueryRDF`. The response would contain
a `Rdf` field, which has the RDF encoded result.

```go
	// Query the balance for Alice and Bob.
	const q = `
		{
			all(func: anyofterms(name, "Alice Bob")) {
				name
				balance
			}
		}
	`
	resp, err := txn.QueryRDF(context.Background(), q)
	if err != nil {
		log.Fatal(err)
	}
 
	// <0x17> <name> "Alice" .
	// <0x17> <balance> 100 .
	fmt.Println(resp.Rdf)
```

## Run a mutation

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

## Commit the transaction

Once all the queries and mutations are done, you can commit the transaction. It
returns an error in case the transaction could not be committed.

```go
	// Finally, we can commit the transactions. An error will be returned if
	// other transactions running concurrently modify the same data that was
	// modified in this transaction. It is up to the library user to retry
	// transactions when they fail.

	err := txn.Commit(context.Background())
```

## Complete Example

This is an example from the [GoDoc](https://godoc.org/github.com/dgraph-io/dgo). It shows how to to create a Node with name Alice, while also creating her relationships with other nodes. Note `loc` predicate is of type `geo` and can be easily marshalled and unmarshalled into a Go struct. More such examples are present as part of the GoDoc.

```go
type School struct {
	Name string `json:"name,omitempty"`
	DType []string `json:"dgraph.type,omitempty"`
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
		DType    []string   `json:"dgraph.type,omitempty"`
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

type Person {
  name
  age
  dob
  married
  raw
  friends
  loc
  school
 }

type Loc {
  type
  coords
 }

type Institution {
  name
 }

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
    Uid:     "_:alice",
	Name:    "Alice",
	DType: []string{"Person"},
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
variables := map[string]string{"$id": assigned.Uids["alice"]}
q := `query Me($id: string){
	me(func: uid($id)) {
		name
		dob
		age
		loc
		raw_bytes
		married
		dgraph.type
		friend @filter(eq(name, "Bob")){
			name
			age
			dgraph.type
		}
		school {
			name
			dgraph.type
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
// Output: {"me":[{"name":"Alice","dob":"1980-01-01T23:00:00Z","age":26,"loc":{"type":"Point","coordinates":[1.1,2]},"raw_bytes":"cmF3X2J5dGVz","married":true,"dgraph.type":["Person"],"friend":[{"name":"Bob","age":24,"dgraph.type":["Person"]}],"school":[{"name":"Crown Public School","dgraph.type":["Institution"]}]}]}


```
