+++
date = "2017-03-20T19:35:35+11:00"
title = "Clients"
+++

## Implementation

Clients can communicate with the server in two different ways:

- **Via [gRPC](http://www.grpc.io/).** Internally this uses [Protocol
  Buffers](https://developers.google.com/protocol-buffers) (the proto file
used by Dgraph is located at
[task.proto](https://github.com/dgraph-io/dgraph/blob/master/protos/task.proto)).

- **Via HTTP.** There are various endpoints, each accepting and returning JSON.
  There is a one to one correspondence between the HTTP endpoints and the gRPC
service methods.


It's possible to interface with dgraph directly via gRPC or HTTP. However, if a
client library exists for you language, this will be an easier option.

## Go

[![GoDoc](https://godoc.org/github.com/dgraph-io/dgraph/client?status.svg)](https://godoc.org/github.com/dgraph-io/dgraph/client)

The go client communicates with the server on the grpc port (set with option `--grpc_port` when starting Dgraph).

The client can be obtained in the usual way via `go get`:

```sh
go get -u -v github.com/dgraph-io/dgraph/client
```

The full [GoDoc](https://godoc.org/github.com/dgraph-io/dgraph/client) contains
documentation for the client API along with examples showing how to use it.

### Create the client

To create a client, dial a connection to Dgraph's external Grpc port (typically
9080). The following code snippet shows just one connection. You can connect to multiple Dgraph serers to distribute the workload evenly.

```go
func newClient() *client.Dgraph {
	// Dial a gRPC connection. The address to dial to can be configured when
	// setting up the dgraph cluster.
	d, err := grpc.Dial("localhost:9080", grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}

	return client.NewDgraphClient(
		protos.NewDgraphClient(d),
	)
}
```

### Alter the database

To set the schema, set it on a `protos.Operation` object, and pass it down to
the `Alter` method.

```go
func setup(c *client.Dgraph) {
	// Install a schema into dgraph. Accounts have a `name` and a `balance`.
	err := c.Alter(ctx, &protos.Operation{
		Schema: `
			name: string @index(term) .
			balance: int .
		`,
	})
}
```

`protos.Operation` contains other fields as well, including drop predicate and
drop all. Drop all is useful if you wish to discard all the data, and start from
a clean slate, without bringing the instance down.

```go
	// Drop all data including schema from the dgraph instance. This is useful
	// for small examples such as this, since it puts dgraph into a clean
	// state.
	err := c.Alter(ctx, &protos.Operation{DropAll: true})
```

### Create a transaction

Dgraph v0.9 supports running distributed ACID transactions. To create a
transaction, just call `c.NewTxn()`. This operation incurs no network call.
Typically, you'd also want to call a `defer txn.Discard()` to let it
automatically rollback in case of errors. Calling `Discard` after `Commit` would
be a no-op.

```go
func runTxn(c *client.Dgraph) {
  txn := c.NewTxn()
  defer txn.Discard()
}
```

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

`txn.Mutate` would run the mutation. It takes in a `protos.Mutation` object,
which provides two main ways to set data: JSON and RDF N-Quad. You can choose
whichever way is convenient.

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

  _, err := txn.Mutate(ctx, &protos.Mutation{SetJSON: out})
```

Sometimes, you only want to commit mutation, without querying anything further.
In such cases, you can use a `CommitImmediately` field in `protos.Mutation` to
indicate that the mutation must be immediately committed.

### Commit the transaction

Once all the queries and mutations are done, you can commit the transaction. It
returns an error in case the transaction could not be committed.

```go
	// Finally, we can commit the transactions. An error will be returned if
	// other transactions running concurrently modify the same data that was
	// modified in this transaction. It is up to the library user to retry
	// transactions when they fail.

  err := txn.Commit(ctx)
```

## Java

The Java client is a new and fully supported client for v0.9.0.

The client can be found [here](https://github.com/dgraph-io/dgraph4j).

## Javascript

{{% notice "note" %}}
A Javascript client doesn't exist yet. But due to popular demand, a Javascript
client will be created to work with dgraph v0.9.0. Watch this space!
{{% /notice %}}

## Python
{{% notice "incomplete" %}}
A lot of development has gone into the Go client and the Python client is not up to date with it.
The Python client is not compatible with dgraph v0.9.0 and onwards.
We are looking for help from contributors to bring it up to date.
{{% /notice %}}

The Python client can be found [here](https://github.com/dgraph-io/pydgraph).

## Raw HTTP

It's also possible to interact with dgraph directly from the command line via
its HTTP endpoints.

To do this, regular command line tools such as `curl` can be used.

The example here uses a simple banking system, where each account has a name
and balance. The operations performed are displaying all balances and
transferring money between accounts.

The following commands assume that dgraph is running locally and is listening
for HTTP on port 8080 (this is the default port to listen on, but can be
changed using the (`--port_offset` flag).

See [Getting Started]({{< relref "get-started/index.md" >}}) for instructions on
how to start up a dgraph instance.

### Setting the schema

The `/alter` endpoint is used to create the schema. Here, the predicate `name`
is the name of an account. It's indexed so that we can look up accounts based
on their name.

```sh
curl -X POST localhost:8080/alter -d 'name: string @index(term) .'
```

If all goes well, the response should be `{"code":"Success","message":"Done"}`.

### Adding initial data

Next we want to add some accounts and an initial balance. To modify or add
data, the `/mutate` endpoint can be used.

{{% notice "note" %}}
The `$'...'` is used to preserve newlines in the body. This is important to do
for the mutate endpoint, since newlines are part of the RDF syntax.
{{% /notice %}}

The `X-Dgraph-CommitNow` header tells dgraph that the mutation is to be
committed immediately as a stand-alone unit. It's not part of a larger
transaction.

```sh
curl -X POST -H 'X-Dgraph-CommitNow: true' localhost:8080/mutate -d $'
{
  set {
    _:alice <name> "Alice" .
    _:alice <balance> "100" .
    _:bob <name> "Bob" .
    _:bob <balance> "70" .
  }
}
'
```

If all goes well, the response will include `"code": "Success"`, and look something like:

```json
{
  "data": {
    "code": "Success",
    "message": "Done",
    "uids": {
      "alice": "0x3",
      "bob": "0x4"
    }
  },
  "extensions": {
    "txn": {
      "start_ts": 9,
      "lin_read": {
        "ids": {
          "1": 18
        }
      }
    }
  }
}
```

### Performing Queries

To query the database, the `/query` endpoint is used. To get the balances for
both accounts:

```sh
curl -X POST localhost:8080/query -d $'
{
  balances(func: anyofterms(name, "Alice Bob")) {
    uid
    name
    balance
  }
}
'
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
      "parsing_ns": 12235,
      "processing_ns": 156547,
      "encoding_ns": 404217
    },
    "txn": {
      "start_ts": 4,
      "lin_read": {
        "ids": {
          "1": 12
        }
      }
    }
  }
}
```

### Transactions

Any transfer of funds should be done as a transaction, to avoid problems such
as [double spending](https://en.wikipedia.org/wiki/Double-spending) (among
others).

First, the account balances of the relevant accounts must be queried. Then then
mutations must be submitted based on the query results. All of this must occur
within a single transaction.

The query response from the previous query can be used. In particular, we
need the `"start_ts"` field and the uids of Alice and Bob.

Say we wish to transfer $10 from Bob to Alice. Based on the result of the
previous query, Alice's balance should become $110 and Bob's balance should
become $60.

The `X-Dgraph-StartTs` header should match the `"start_ts"` returned from the
first query in the transaction.

The `X-Dgraph-CommitNow` header isn't needed since the mutation is part of a
larger transaction.

```sh
curl -X POST -H 'X-Dgraph-StartTs: 4' localhost:8080/mutate -d $'
{
  set {
    <0x1> <balance> "110" .
    <0x2> <balance> "60" .
  }
}
'
```

The result should look like:

```json
{
  "data": {
    "code": "Success",
    "message": "Done",
    "uids": {}
  },
  "extensions": {
    "txn": {
      "start_ts": 4,
      "keys": [
        "AAALX3ByZWRpY2F0ZV8AAAAAAAAAAAE=",
        "AAAHYmFsYW5jZQAAAAAAAAAAAg==",
        "AAAHYmFsYW5jZQAAAAAAAAAAAQ==",
        "AAALX3ByZWRpY2F0ZV8AAAAAAAAAAAI="
      ],
      "lin_read": {
        "ids": {
          "1": 15
        }
      }
    }
  }
}
```

To finally commit the transaction, the `/commit` endpoint is used. The
`start_ts` from the original query, along with the keys from all mutations (in
this case, just one mutation) must be supplied.

```sh
curl -X POST -H 'X-Dgraph-StartTs: 4' -H 'X-Dgraph-Keys: ["AAALX3ByZWRpY2F0ZV8AAAAAAAAAAAE=","AAAHYmFsYW5jZQAAAAAAAAAAAg==","AAAHYmFsYW5jZQAAAAAAAAAAAQ==","AAALX3ByZWRpY2F0ZV8AAAAAAAAAAAI="]' localhost:8080/commit 
```

If the commit is successful (which it should be in this example), the result
will be:

```json
{
  "data": {
    "code": "Success",
    "message" :"Done",
  },
  "extensions": {
    "txn": {
      "start_ts": 4
      "commit_ts": 5,
    }
  }
}
```

If there were any mutations effecting any relevant keys after `start_ts` but
before the completion of the transaction, the commit will fail. For example:

```json
{
  "errors": [
    {
      "code": "Error",
      "message": "Transaction aborted"
    }
  ]
}
```
