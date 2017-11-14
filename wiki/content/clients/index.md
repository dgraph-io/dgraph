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
	...
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

To do this, regular command line tools such as `curl` and
[`jq`](https://stedolan.github.io/jq/) can be used.

Similar to the Go client example, we use a bank account transfer example.

The following commands assume that dgraph is running locally and is listening
for HTTP on port 8080 (this is the default port to listen on, but can be
changed using the (`--port_offset` flag).

### Alter the database

The `/alter` endpoint is used to create or change the schema. Here, the
predicate `name` is the name of an account. It's indexed so that we can look up
accounts based on their name.

```sh
curl -X POST localhost:8080/alter -d 'name: string @index(term) .'
```

If all goes well, the response should be `{"code":"Success","message":"Done"}`.

### Start a transaction

The state associated with a transaction consists of a start time stamp
(`start_ts`), a linearized read map (`lin_map`), and a set of keys affected by
the transaction (`keys`). The client (or user for raw HTTP) needs to keep track of this
state themselves.

The `start_ts` is the start time of the transaction, as provided by dgraph
zero. It acts to uniquely identify a transaction, and doesn't change over the
transaction lifetime.

The `lin_read` is a map from dgraph group to proposal id. A new `lin_read` is
returned by all interactions with dgraph. Whenever a new `lin_read` is
received, it should be merged with the existing `lin_read` map. The merge
operation should add new keys when they don't already exist or otherwise take
the maximum of the new value and the existing value. The client's `lin_read`
should be sent to dgraph along side all queries.

The `keys` are the set of keys that have been read or written to in the
transaction, and is used by dgraph to check for conflicts with other
transactions.

### Adding initial data

The first transaction to perform is to add two accounts with initial balances.
To do this, we use the `/mutate` endpoint. The endpoint accepts mutations in
RDF format.

{{% notice "note" %}}
The `$'...'` is used to preserve newlines in the body. This is important to do
for the mutate endpoint, since newlines are part of the RDF syntax.
{{% /notice %}}

{{% notice "note" %}}
The `X-Dgraph-CommitNow` header can be used with the `/mutate` endpoint to
force a mutation to commit immediately. This can be convenient if a single
mutation with no queries is needed. This approach isn't the normal approach, and
isn't used in this example.
{{% /notice %}}

```sh
curl -X POST localhost:8080/mutate -d $'
{
  set {
    _:alice <name> "Alice" .
    _:alice <balance> "100" .
    _:bob <name> "Bob" .
    _:bob <balance> "70" .
  }
}
' | jq
```

If all goes well, the response will include `"code": "Success"`.

```json
{
  "data": {
    "code": "Success",
    "message": "Done",
    "uids": {
      "alice": "0x2",
      "bob": "0x1"
    }
  },
  "extensions": {
    "txn": {
      "start_ts": 2,
      "keys": [
        "AAAHYmFsYW5jZQAAAAAAAAAAAQ==",
        "AAAEbmFtZQAAAAAAAAAAAQ==",
        "AAAHYmFsYW5jZQAAAAAAAAAAAg==",
        "AAAEbmFtZQAAAAAAAAAAAg==",
        "AAALX3ByZWRpY2F0ZV8AAAAAAAAAAAI=",
        "AAALX3ByZWRpY2F0ZV8AAAAAAAAAAAI=",
        "AAALX3ByZWRpY2F0ZV8AAAAAAAAAAAE=",
        "AAAEbmFtZQIBYm9i",
        "AAALX3ByZWRpY2F0ZV8AAAAAAAAAAAE=",
        "AAAEbmFtZQIBYWxpY2U="
      ],
      "lin_read": {
        "ids": {
          "1": 12
        }
      }
    }
  }
}
```

The important fields to look at are `start_ts`, `keys`, and `lin_read`.
`start_ts` will be needed for any further interactions with the transaction,
`keys` will be needed when the transaction is committed, and `lin_read` will be
needed when doing any queries in the transaction.

We now want to commit the transaction, so we need `start_ts` and `keys`. To do
this, we can use the `/commit` endpoint. The `X-Dgraph-StartTs` and
`X-Dgraph-Keys` headers are based on the response above (and could differ when
you run the example).

```sh
curl -X POST -H 'X-Dgraph-StartTs: 2' -H 'X-Dgraph-Keys: ["AAAHYmFsYW5jZQAAAAAAAAAAAQ==","AAAEbmFtZQAAAAAAAAAAAQ==","AAAHYmFsYW5jZQAAAAAAAAAAAg==","AAAEbmFtZQAAAAAAAAAAAg==","AAALX3ByZWRpY2F0ZV8AAAAAAAAAAAI=","AAALX3ByZWRpY2F0ZV8AAAAAAAAAAAI=","AAALX3ByZWRpY2F0ZV8AAAAAAAAAAAE=","AAAEbmFtZQIBYm9i","AAALX3ByZWRpY2F0ZV8AAAAAAAAAAAE=","AAAEbmFtZQIBYWxpY2U="]' localhost:8080/commit | jq
```

If the commit is successful, the result should be:

```json
{
  "data": {
    "code": "Success",
    "message": "Done"
  },
  "extensions": {
    "txn": {
      "start_ts": 2,
      "commit_ts": 3
    }
  }
}
```

Note that a `commit_ts` is included in the response. This indicates that the
transaction is now finished (in previous responses from dgraph, this field has
been absent).

### Performing queries

We now want to transfer money from one account to the other. This is done in
three steps:

1. Run a query to determine the current balances. This is the start of the
   transaction.

2. Perform a mutation to update the balances.

3. Commit the transaction.

To query the database, the `/query` endpoint is used. We need to provide the
`lin_read` we received in our original mutation.

To get the balances for both accounts:

```sh
curl -X POST -H 'X-Dgraph-LinRead: {"1": 12}' localhost:8080/query -d $'
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
      "lin_read": {
        "ids": {
          "1": 14
        }
      }
    }
  }
}
```

### Mutations based on queries

Now that we have the current balances, we need to send a mutation to dgraph
with the updated balances. If Bob transfers $10 to Alice, then the RDFs to send
are:

```
<0x1> <balance> "110" .
<0x2> <balance> "60" .

```
Note that to refer to the Alice and Bob nodes, we use the UIDs returned
by the query.

We now send the mutations via the `/mutate` endpoint. We need to provide the
transaction start timestamp via the header, so that dgraph knows which
transaction the mutation should be part of. The `start_ts` from the query we
just ran should be used.

```sh
curl -X POST -H 'X-Dgraph-StartTs: 4' localhost:8080/mutate -d $'
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
    "txn": {
      "start_ts": 4,
      "keys": [
        "AAALX3ByZWRpY2F0ZV8AAAAAAAAAAAI=",
        "AAAHYmFsYW5jZQAAAAAAAAAAAg==",
        "AAALX3ByZWRpY2F0ZV8AAAAAAAAAAAE=",
        "AAAHYmFsYW5jZQAAAAAAAAAAAQ=="
      ],
      "lin_read": {
        "ids": {
          "1": 17
        }
      }
    }
  }
}
```

### Committing the transaction

Finally, we can commit the transaction using the `/commit` endpoint. We need
the `start_ts` we've been using for the transaction, and the affected `keys`.
If we had performed multiple mutations in the transaction instead of the just
the one, then the keys provided during the commit should be the union of all
keys returned in the responses from the `/mutate` endpoint.

```sh
curl -X POST -H 'X-Dgraph-StartTs: 4' -H 'X-Dgraph-Keys: ["AAALX3ByZWRpY2F0ZV8AAAAAAAAAAAI=","AAAHYmFsYW5jZQAAAAAAAAAAAg==","AAALX3ByZWRpY2F0ZV8AAAAAAAAAAAE=","AAAHYmFsYW5jZQAAAAAAAAAAAQ=="]' localhost:8080/commit | jq
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
If other clients are performing transactions concurrently that affect the same
keys, then it's possible that the transaction will *not* be successful. This is
indicated in the response when the commit is attempted.

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

In this case, it's up to the user of the client to decide if they wish to
retry the transaction.

Assuming that the transaction was successful, we can do one final query to show
the new balances. We need to provide an updated `lin_read` map when we do the
query, otherwise we might old data (this can happen if a replica is slightly
behind). The last `lin_read` maps we have received so far are `{"1": 12}`,
`{"1": 14}`, and `{"1": 17}`. They all have the same key, so merging them is
easy. We just use the maximum value to give `{"1": 17}`.

```sh
curl -X POST -H 'X-Dgraph-LinRead: {"1": 17}' localhost:8080/query -d $'
{
  balances(func: anyofterms(name, "Alice Bob")) {
    name
    balance
  }
}' | jq
```

```json
{
  "data": {
    "balances": [
      {
        "name": "Alice",
        "balance": "110"
      },
      {
        "name": "Bob",
        "balance": "60"
      }
    ]
  },
  "extensions": {
    "server_latency": {
      "parsing_ns": 27038,
      "processing_ns": 340099,
      "encoding_ns": 941622
    },
    "txn": {
      "start_ts": 6,
      "lin_read": {
        "ids": {
          "1": 20
        }
      }
    }
  }
}

```
