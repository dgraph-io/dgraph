+++
date = "2017-03-20T22:25:17+11:00"
title = "HTTP Client"
weight = 5
[menu.main]
    identifier = "js-http-client"
    parent = "javascript"
+++

A Dgraph client implementation for JavaScript using HTTP. It supports both
browser and Node.js environments.
This client follows the [Dgraph JavaScript gRPC client]({{< relref "grpc.md" >}}) closely.

{{% notice "tip" %}}
The official JavaScript HTTP client [can be found here](https://github.com/dgraph-io/dgraph-js-http).
Follow the [install instructions](https://github.com/dgraph-io/dgraph-js-http#install) to get it up and running.
{{% /notice %}}

## Supported Versions

More details on the supported versions can be found at [this link](https://github.com/dgraph-io/dgraph-js-http#supported-versions).

## Quickstart

Build and run the [simple project](https://github.com/dgraph-io/dgraph-js-http/tree/master/examples/simple), which
contains an end-to-end example of using the Dgraph javascript HTTP client. Follow
the instructions in the [README](https://github.com/dgraph-io/dgraph-js-http/tree/master/examples/simple/README.md) of that project.

## Using a client

{{% notice "tip" %}}
You can find a [simple example](https://github.com/dgraph-io/dgraph-js-http/tree/master/examples/simple)
project, which contains an end-to-end working example of how to use the JavaScript HTTP client,
for Node.js >= v6.
{{% /notice %}}

### Create a client

A `DgraphClient` object can be initialised by passing it a list of
`DgraphClientStub` clients as variadic arguments. Connecting to multiple Dgraph
servers in the same cluster allows for better distribution of workload.

The following code snippet shows just one connection.

```js
const dgraph = require("dgraph-js-http");

const clientStub = new dgraph.DgraphClientStub(
    // addr: optional, default: "http://localhost:8080"
    "http://localhost:8080",
    // legacyApi: optional, default: false. Set to true when connecting to Dgraph v1.0.x
    false,
);
const dgraphClient = new dgraph.DgraphClient(clientStub);
```

To facilitate debugging, [debug mode](#debug-mode) can be enabled for a client.

### Login into Dgraph

If your Dgraph server has Access Control Lists enabled (Dgraph v1.1 or above),
the clientStub must be logged in for accessing data:

```js
await clientStub.login("groot", "password");
```

Calling `login` will obtain and remember the access and refresh JWT tokens.
All subsequent operations via the logged in `clientStub` will send along the
stored access token.

Access tokens expire after 6 hours, so in long-lived apps (e.g. business logic servers)
you need to `login` again on a periodic basis:

```js
// When no parameters are specified the clientStub uses existing refresh token
// to obtain a new access token.
await clientStub.login();
```

### Configure access tokens

Some Dgraph configurations require extra access tokens.

1. Alpha servers can be configured with [Secure Alter Operations](https://dgraph.io/docs/deploy/dgraph-administration/#securing-alter-operations).
   In this case the token needs to be set on the client instance:

```js
dgraphClient.setAlphaAuthToken("My secret token value");
```

2. [Slash GraphQL](https://dgraph.io/slash-graphql) requires API key for HTTP access:

```js
dgraphClient.setSlashApiKey("Copy the Api Key from Slash GraphQL admin page");
```

### Create https connection

If your cluster is using tls/mtls you can pass a node `https.Agent` configured with you
certificates as follows:

```js
const https = require("https");
const fs = require("fs");
// read your certificates
const cert = fs.readFileSync("./certs/client.crt", "utf8");
const ca = fs.readFileSync("./certs/ca.crt", "utf8");
const key = fs.readFileSync("./certs/client.key", "utf8");

// create your https.Agent
const agent = https.Agent({
    cert,
    ca,
    key,
});

const clientStub = new dgraph.DgraphClientStub(
    "https://localhost:8080",
    false,
    { agent },
);
const dgraphClient = new dgraph.DgraphClient(clientStub);
```

### Alter the database

To set the schema, pass the schema to `DgraphClient#alter(Operation)` method.

```js
const schema = "name: string @index(exact) .";
await dgraphClient.alter({ schema: schema });
```

> NOTE: Many of the examples here use the `await` keyword which requires
> `async/await` support which is not available in all javascript environments.
> For unsupported environments, the expressions following `await` can be used
> just like normal `Promise` instances.

`Operation` contains other fields as well, including drop predicate and drop all.
Drop all is useful if you wish to discard all the data, and start from a clean
slate, without bringing the instance down.

```js
// Drop all data including schema from the Dgraph instance. This is useful
// for small examples such as this, since it puts Dgraph into a clean
// state.
await dgraphClient.alter({ dropAll: true });
```

### Create a transaction

To create a transaction, call `DgraphClient#newTxn()` method, which returns a
new `Txn` object. This operation incurs no network overhead.

It is good practise to call `Txn#discard()` in a `finally` block after running
the transaction. Calling `Txn#discard()` after `Txn#commit()` is a no-op
and you can call `Txn#discard()` multiple times with no additional side-effects.

```js
const txn = dgraphClient.newTxn();
try {
    // Do something here
    // ...
} finally {
    await txn.discard();
    // ...
}
```

You can make queries read-only and best effort by passing `options` to `DgraphClient#newTxn`. For example:

```js
const options = { readOnly: true, bestEffort: true };
const res = await dgraphClient.newTxn(options).query(query);
```

Read-only transactions are useful to increase read speed because they can circumvent the usual consensus protocol. Best effort queries can also increase read speed in read bound system. Please note that best effort requires readonly.

### Run a mutation

`Txn#mutate(Mutation)` runs a mutation. It takes in a `Mutation` object, which
provides two main ways to set data: JSON and RDF N-Quad. You can choose whichever
way is convenient.

We define a person object to represent a person and use it in a `Mutation` object.

```js
// Create data.
const p = {
    name: "Alice",
};

// Run mutation.
await txn.mutate({ setJson: p });
```

For a more complete example with multiple fields and relationships, look at the
[simple] project in the `examples` folder.

For setting values using N-Quads, use the `setNquads` field. For delete mutations,
use the `deleteJson` and `deleteNquads` fields for deletion using JSON and N-Quads
respectively.

Sometimes, you only want to commit a mutation, without querying anything further.
In such cases, you can use `Mutation#commitNow = true` to indicate that the
mutation must be immediately committed.

```js
// Run mutation.
await txn.mutate({ setJson: p, commitNow: true });
```

### Run a query

You can run a query by calling `Txn#query(string)`. You will need to pass in a
GraphQL+- query string. If you want to pass an additional map of any variables that
you might want to set in the query, call `Txn#queryWithVars(string, object)` with
the variables object as the second argument.

The response would contain the `data` field, `Response#data`, which returns the response
JSON.

Let’s run the following query with a variable \$a:

```console
query all($a: string) {
  all(func: eq(name, $a))
  {
    name
  }
}
```

Run the query and print out the response:

```js
// Run query.
const query = `query all($a: string) {
  all(func: eq(name, $a))
  {
    name
  }
}`;
const vars = { $a: "Alice" };
const res = await dgraphClient.newTxn().queryWithVars(query, vars);
const ppl = res.data;

// Print results.
console.log(`Number of people named "Alice": ${ppl.all.length}`);
ppl.all.forEach(person => console.log(person.name));
```

This should print:

```console
Number of people named "Alice": 1
Alice
```

### Commit a transaction

A transaction can be committed using the `Txn#commit()` method. If your transaction
consisted solely of calls to `Txn#query` or `Txn#queryWithVars`, and no calls to
`Txn#mutate`, then calling `Txn#commit()` is not necessary.

An error will be returned if other transactions running concurrently modify the same
data that was modified in this transaction. It is up to the user to retry
transactions when they fail.

```js
const txn = dgraphClient.newTxn();
try {
    // ...
    // Perform any number of queries and mutations
    // ...
    // and finally...
    await txn.commit();
} catch (e) {
    if (e === dgraph.ERR_ABORTED) {
        // Retry or handle exception.
    } else {
        throw e;
    }
} finally {
    // Clean up. Calling this after txn.commit() is a no-op
    // and hence safe.
    await txn.discard();
}
```

### Check request latency

To see the server latency information for requests, check the
`extensions.server_latency` field from the Response object for queries or from
the Assigned object for mutations. These latencies show the amount of time the
Dgraph server took to process the entire request. It does not consider the time
over the network for the request to reach back to the client.

```js
// queries
const res = await txn.queryWithVars(query, vars);
console.log(res.extensions.server_latency);
// { parsing_ns: 29478,
//  processing_ns: 44540975,
//  encoding_ns: 868178 }

// mutations
const assigned = await txn.mutate({ setJson: p });
console.log(assigned.extensions.server_latency);
// { parsing_ns: 132207,
//   processing_ns: 84100996 }
```

### Debug mode

Debug mode can be used to print helpful debug messages while performing alters,
queries and mutations. It can be set using the`DgraphClient#setDebugMode(boolean?)`
method.

```js
// Create a client.
const dgraphClient = new dgraph.DgraphClient(...);

// Enable debug mode.
dgraphClient.setDebugMode(true);
// OR simply dgraphClient.setDebugMode();

// Disable debug mode.
dgraphClient.setDebugMode(false);
```
