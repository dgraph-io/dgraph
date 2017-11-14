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
documentation for the client API as well as some more examples.

A full example is shown here. In the example, dgraph stores some financial
accounts, each with a name and a balance. The example works as follows:

1. Create a dgraph client.
2. Set up a schema.
3. Add some initial data into dgraph. There are two accounts, Alice has $100
   and Bob has $70.
4. Query the data to see the initial account balances.
5. Perform a transaction on the data. $10 is transferred from Bob to Alice.
6. View the account balances again (this will reflected the transfered amount).

These steps correspond to each of the funciton calls from `func main()`.

Here is the program:

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/protos"
	"google.golang.org/grpc"
)

var ctx = context.Background()

func main() {
	c := newClient()
	setup(c)
	initData(c)
	makeQuery(c)
	doTxn(c)
	makeQuery(c)
}

func newClient() *client.Dgraph {
	// Dial a gRPC connection. The address to dial to can be configured when
	// setting up the dgraph cluster.
	d, err := grpc.Dial("localhost:19080", grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}

	return client.NewDgraphClient(
		protos.NewDgraphClient(d),
	)
}

func setup(c *client.Dgraph) {
	// Drop all data including schema from the dgraph instance. This is useful
	// for small examples such as this, since it puts dgraph into a clean
	// state.
	if err := c.Alter(ctx, &protos.Operation{
		DropAll: true,
	}); err != nil {
		log.Fatal(err)
	}

	// Install a schema into dgraph. Accounts have a `name` and a `balance`.
	if err := c.Alter(ctx, &protos.Operation{
		Schema: `
			name: string @index(term) .
			balance: int .
		`,
	}); err != nil {
		log.Fatal(err)
	}
}

func initData(c *client.Dgraph) {
	// Set some initial data. We initially give Alice $100 and Bob $70.
	txn := c.NewTxn()
	defer txn.Discard(ctx)
	if _, err := txn.Mutate(ctx, &protos.Mutation{

		// By setting CommitImmediately to true, we don't have to call
		// txn.Commit for the data to be persisted.
		CommitImmediately: true,

		// We're using NQuads in this example, but JSON could also be used by
		// setting the SetJson field in protos.Mutation.
		SetNquads: []byte(`
			_:alice <name> "Alice" .
			_:alice <balance> "100" .
			_:bob <name> "Bob" .
			_:bob <balance> "70" .
		`),
	}); err != nil {
		log.Fatal(err)
	}
}

func makeQuery(c *client.Dgraph) {
	// Query the details for all accounts. The response is in JSON format.
	resp, err := c.NewTxn().Query(context.Background(), `
		{
		  balances(func: anyofterms(name, "Alice Bob")) {
			name
			balance
		  }
		}
	`)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(resp.GetJson()))
}

func doTxn(c *client.Dgraph) {
	// We wish to transfer $10 from Bob to Alice. This should occur in a
	// transaction, since we don't want other users of dgraph to see an
	// intermediate state.

	// First query the balance for Alice and Bob.
	const q = `
		{
		  alice(func: anyofterms(name, "Alice")) {
			uid
			balance
		  }
		  bob(func: anyofterms(name, "Bob")) {
			uid
			balance
		  }
		}
	`
	txn := c.NewTxn()
	defer txn.Discard(ctx)
	resp, err := txn.Query(context.Background(), q)
	if err != nil {
		log.Fatal(err)
	}

	// After we get the balances, we have to decode them into structs so that
	// we can manipulate the data.
	var decode struct {
		Alice []struct {
			Uid     string
			Balance int
		}
		Bob []struct {
			Uid     string
			Balance int
		}
	}
	if err := json.Unmarshal(resp.GetJson(), &decode); err != nil {
		log.Fatal(err)
	}

	// Now we can create the new mutations with the adjusted account balances.
	rdfs := fmt.Sprintf(`
		<%s> <balance> "%d" .
		<%s> <balance> "%d" .
	`,
		decode.Alice[0].Uid, decode.Alice[0].Balance+10,
		decode.Bob[0].Uid, decode.Bob[0].Balance-10,
	)
	if _, err := txn.Mutate(ctx, &protos.Mutation{
		SetNquads: []byte(rdfs)},
	); err != nil {
		log.Fatal(err)
	}

	// Finally, we can commit the transactions. An error will be returned if
	// other transactions running concurrently modify the same data that was
	// modified in this transaction. It is up to the library user to retry
	// transactions when they fail.
	if err := txn.Commit(ctx); err != nil {
		log.Fatal(err)
	}
}
```

And its output:

```
{
  "balances": [
    {
      "name": "Alice",
      "balance": 100
    },
    {
      "name": "Bob",
      "balance": 70
    }
  ]
}
{
  "balances": [
    {
      "name": "Alice",
      "balance": 110
    },
    {
      "name": "Bob",
      "balance": 60
    }
  ]
}
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
changed using the (`--port` flag).

See [Getting Started](http://localhost:1313/get-started/) for instructions on
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

```
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
      "keys": [
        "\u0000\u0000\u0007balance\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0004",
        "\u0000\u0000\u0004name\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0004",
        "\u0000\u0000\u0004name\u0002\u0002Bob",
        "\u0000\u0000\u0004name\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0003",
        "\u0000\u0000\u000b_predicate_\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0004",
        "\u0000\u0000\u0007balance\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0003",
        "\u0000\u0000\u0004name\u0002\u0002Alice",
        "\u0000\u0000\u000b_predicate_\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0003",
        "\u0000\u0000\u000b_predicate_\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0004",
        "\u0000\u0000\u000b_predicate_\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0003"
      ],
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

```
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

The `X-Dgraph-StartTs` header should match the `"read_ts"` returned from the
first query in the transaction.

The `X-Dgraph-CommitNow` header isn't needed since the mutation is part of a
larger transaction.

```
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

```
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
        "\u0000\u0000\u000b_predicate_\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0002",
        "\u0000\u0000\u0007balance\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0001",
        "\u0000\u0000\u000b_predicate_\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0001",
        "\u0000\u0000\u0007balance\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0002"
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
curl -X POST -H 'X-Dgraph-StartTs: 4' -H 'X-Dgraph-Keys: ["\u0000\u0000\u000b_predicate_\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0002","\u0000\u0000\u0007balance\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0001","\u0000\u0000\u000b_predicate_\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0001","\u0000\u0000\u0007balance\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0002"]' localhost:8080/commit 
```

If the commit is successful (which it should be in this example), the result
will be:

```
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

```
{
  "errors": [
    {
      "code": "Error",
      "message": "Transaction aborted"
    }
  ]
}
```
