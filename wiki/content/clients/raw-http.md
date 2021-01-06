+++
date = "2017-03-20T22:25:17+11:00"
title = "Raw HTTP"
weight = 8
[menu.main]
    parent = "clients"
+++

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

## Create the Client

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

## Alter the database

The `/alter` endpoint is used to create or change the schema. Here, the
predicate `name` is the name of an account. It's indexed so that we can look up
accounts based on their name.

```sh
$ curl -X POST localhost:8080/alter -d \
'name: string @index(term) .
type Person {
   name
}'
```

If all goes well, the response should be `{"code":"Success","message":"Done"}`.

Other operations can be performed via the `/alter` endpoint as well. A specific
predicate or the entire database can be dropped.

To drop the predicate `name`:

```sh
$ curl -X POST localhost:8080/alter -d '{"drop_attr": "name"}'
```

To drop the type `Film`:
```sh
$ curl -X POST localhost:8080/alter -d '{"drop_op": "TYPE", "drop_value": "Film"}'
```

To drop all data and schema:
```sh
$ curl -X POST localhost:8080/alter -d '{"drop_all": true}'
```

To drop all data only (keep schema):
```sh
$ curl -X POST localhost:8080/alter -d '{"drop_op": "DATA"}'
```

## Start a transaction

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

## Run a query

To query the database, the `/query` endpoint is used. Remember to set the `Content-Type` header
to `application/dql` to ensure that the body of the request is parsed correctly.

{{% notice "note" %}}
GraphQL+- has been renamed to Dgraph Query Language (DQL). While `application/dql`
is the preferred value for the `Content-Type` header, we will continue to support
`Content-Type: application/graphql+-` to avoid making breaking changes.
{{% /notice %}}

To get the balances for both accounts:

```sh
$ curl -H "Content-Type: application/dql" -X POST localhost:8080/query -d $'
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

## Run a Mutation

Now that we have the current balances, we need to send a mutation to Dgraph
with the updated balances. If Bob transfers $10 to Alice, then the RDFs to send
are:

```
<0x1> <balance> "110" .
<0x1> <dgraph.type> "Balance" .
<0x2> <balance> "60" .
<0x2> <dgraph.type> "Balance" .
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
    <0x1> <dgraph.type> "Balance" .
    <0x2> <balance> "60" .
    <0x2> <dgraph.type> "Balance" .
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
      "parsing_ns": 50901,
      "processing_ns": 14631082
    },
    "txn": {
      "start_ts": 4,
      "keys": [
        "2ahy9oh4s9csc",
        "3ekeez23q5149"
      ],
      "preds": [
        "1-balance"
      ]
    }
  }
}
```

We get some `keys`. These should be added to the set of `keys` stored in the
transaction state. We also get some `preds`, which should be added to the set of
`preds` stored in the transaction state.

## Committing the transaction

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
		"2ahy9oh4s9csc",
		"3ekeez23q5149"
	],
  "preds": [
    "1-balance"
	]
}' | jq
```

The result:

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

## Aborting the transaction
To abort a transaction, use the same `/commit` endpoint with the `abort=true` parameter
while specifying the `startTs` value for the transaction.

```sh
$ curl -X POST "localhost:8080/commit?startTs=4&abort=true" | jq
```

The result:

```json
{
  "code": "Success",
  "message": "Done"
}
```

## Running read-only queries

You can set the query parameter `ro=true` to `/query` to set it as a
[read-only]({{< relref "clients/go.md#read-only-transactions" >}}) query.


```sh
$ curl -H "Content-Type: application/dql" -X POST "localhost:8080/query?ro=true" -d $'
{
  balances(func: anyofterms(name, "Alice Bob")) {
    uid
    name
    balance
  }
}
```

## Running best-effort queries

You can set the query parameter `be=true` to `/query` to set it as a
[best-effort]({{< relref "clients/go.md#read-only-transactions" >}}) query.


```sh
$ curl -H "Content-Type: application/dql" -X POST "localhost:8080/query?be=true" -d $'
{
  balances(func: anyofterms(name, "Alice Bob")) {
    uid
    name
    balance
  }
}
```

## Compression via HTTP

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
  -H "Content-Type: application/dql" \
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
  -H "Content-Type: application/dql" \
  localhost:8080/query --data-binary @query.gz | gzip --decompress
```

{{% notice "note" %}}
Curl has a `--compressed` option that automatically requests for a compressed response (`Accept-Encoding` header) and decompresses the compressed response.

```sh
$ curl -X POST --compressed -H "Content-Type: application/dql" localhost:8080/query -d $'schema {}'
```
{{% /notice %}}


## Run a query in JSON format

The HTTP API also accepts requests in JSON format. For queries you have the keys "query" and "variables". The JSON format is required to set [GraphQL Variables]({{< relref "query-language/graphql-variables.md" >}}) with the HTTP API.

This query:

```
{
  balances(func: anyofterms(name, "Alice Bob")) {
    uid
    name
    balance
  }
}
```

Should be escaped to this:

```sh
curl -H "Content-Type: application/json" localhost:8080/query -XPOST -d '{
    "query": "{\n balances(func: anyofterms(name, \"Alice Bob\")) {\n uid\n name\n balance\n }\n }"
}' | python -m json.tool | jq
```
