+++
date = "2017-03-20T22:25:17+11:00"
title = "gRPC Client"
weight = 5
[menu.main]
    parent = "javascript"
+++

The official Dgraph client implementation for JavaScript (Node.js v6 and above),
using [gRPC](https://grpc.io/).
This client follows the [Dgraph Go client]({{< relref "go.md" >}}) closely.


{{% notice "tip" %}}
The official JavaScript gRPC client [can be found here](https://github.com/dgraph-io/dgraph-js).
Follow the [install instructions](https://github.com/dgraph-io/dgraph-js#install) to get it up and running.
{{% /notice %}}

## Supported Versions

More details on the supported versions can be found at [this link](https://github.com/dgraph-io/dgraph-js#supported-versions).

## Quickstart

Build and run the [simple project](https://github.com/dgraph-io/dgraph-js/tree/master/examples/simple), which
contains an end-to-end example of using the Dgraph JavaScript client. Follow the
instructions in the [README](https://github.com/dgraph-io/dgraph-js/tree/master/examples/simple/README.md) of that project.

### Examples

- [simple](https://github.com/dgraph-io/dgraph-js/tree/master/examples/simple): Quickstart example of using dgraph-js.
- [tls](https://github.com/dgraph-io/dgraph-js/tree/master/examples/tls): Example of using dgraph-js with a Dgraph cluster secured with TLS.

## Using a Client

{{% notice "tip" %}}
You can find a [simple example](https://github.com/dgraph-io/dgraph-js/tree/master/examples/simple)
project, which contains an end-to-end working example of how to use the JavaScript gRPC client,
for Node.js >= v6.
{{% /notice %}}

### Creating a Client

A `DgraphClient` object can be initialised by passing it a list of
`DgraphClientStub` clients as variadic arguments. Connecting to multiple Dgraph
servers in the same cluster allows for better distribution of workload.

The following code snippet shows just one connection.

```js
const dgraph = require("dgraph-js");
const grpc = require("grpc");

const clientStub = new dgraph.DgraphClientStub(
  // addr: optional, default: "localhost:9080"
  "localhost:9080",
  // credentials: optional, default: grpc.credentials.createInsecure()
  grpc.credentials.createInsecure(),
);
const dgraphClient = new dgraph.DgraphClient(clientStub);
```

To facilitate debugging, [debug mode](#debug-mode) can be enabled for a client.

### Altering the Database

To set the schema, create an `Operation` object, set the schema and pass it to
`DgraphClient#alter(Operation)` method.

```js
const schema = "name: string @index(exact) .";
const op = new dgraph.Operation();
op.setSchema(schema);
await dgraphClient.alter(op);
```

Starting Dgraph version 20.03.0, indexes can be computed in the background.
You can set `setRunInBackground` field of the `Operation` object to `true`
before passing it to the `DgraphClient#alter(Operation)` method. You can find more details
[here](https://docs.dgraph.io/master/query-language/#indexes-in-background).

```js
const schema = "name: string @index(exact) .";
const op = new dgraph.Operation();
op.setSchema(schema);
op.setRunInBackground(true);
await dgraphClient.alter(op);
```

> NOTE: Many of the examples here use the `await` keyword which requires
> `async/await` support which is available on Node.js >= v7.6.0. For prior versions,
> the expressions following `await` can be used just like normal `Promise`:
> 
> ```js
> dgraphClient.alter(op)
>     .then(function(result) { ... }, function(err) { ... })
> ```

`Operation` contains other fields as well, including drop predicate and drop all.
Drop all is useful if you wish to discard all the data, and start from a clean
slate, without bringing the instance down.

```js
// Drop all data including schema from the Dgraph instance. This is useful
// for small examples such as this, since it puts Dgraph into a clean
// state.
const op = new dgraph.Operation();
op.setDropAll(true);
await dgraphClient.alter(op);
```

### Creating a Transaction

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

To create a read-only transaction, set `readOnly` boolean to `true` while calling
`DgraphClient#newTxn()` method. Read-only transactions cannot contain mutations and
trying to call `Txn#mutate()` or `Txn#commit()` will result in an error. Calling
`Txn.Discard()` will be a no-op.

You can optionally set the `bestEffort` boolean to `true`. This may yield improved
latencies in read-bound workloads where linearizable reads are not strictly needed.

```js
const txn = dgraphClient.newTxn({
  readOnly: true,
  bestEffort: false
});
// ...
const res = await txn.queryWithVars(query, vars);
```

### Running a Mutation

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
const mu = new dgraph.Mutation();
mu.setSetJson(p);
await txn.mutate(mu);
```

For a more complete example with multiple fields and relationships, look at the
[simple] project in the `examples` folder.

Sometimes, you only want to commit a mutation, without querying anything further.
In such cases, you can use `Mutation#setCommitNow(true)` to indicate that the
mutation must be immediately committed.

`Mutation#setIgnoreIndexConflict(true)` can be applied on a `Mutation` object to
not run conflict detection over the index, which would decrease the number of
transaction conflicts and aborts. However, this would come at the cost of potentially
inconsistent upsert operations.

Mutation can be run using `txn.doRequest` as well.

```js
const mu = new dgraph.Mutation();
mu.setSetJson(p);

const req = new dgraph.Request();
req.setCommitNow(true);
req.setMutationsList([mu]);

await txn.doRequest(req);
```

### Running a Query

You can run a query by calling `Txn#query(string)`. You will need to pass in a
GraphQL+- query string. If you want to pass an additional map of any variables that
you might want to set in the query, call `Txn#queryWithVars(string, object)` with
the variables object as the second argument.

The response would contain the method `Response#getJSON()`, which returns the response
JSON.

Let’s run the following query with a variable $a:

```console
query all($a: string) {
  all(func: eq(name, $a))
  {
    name
  }
}
```

Run the query, deserialize the result from Uint8Array (or base64) encoded JSON and
print it out:

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
const ppl = res.getJson();

// Print results.
console.log(`Number of people named "Alice": ${ppl.all.length}`);
ppl.all.forEach((person) => console.log(person.name));
```

This should print:

```console
Number of people named "Alice": 1
Alice
```

You can also use `txn.doRequest` function to run the query.
```js
const req = new dgraph.Request();
const vars = req.getVarsMap();
vars.set("$a", "Alice");
req.setQuery(query);

const res = await txn.doRequest(req);
console.log(JSON.stringify(res.getJson()));
```

### Running an Upsert: Query + Mutation

The `txn.doRequest` function allows you to run upserts consisting of one query and one mutation. 
Query variables could be defined and can then be used in the mutation. You can also use the 
`txn.doRequest` function to perform just a query or a mutation.

To know more about upsert, we highly recommend going through the docs at https://docs.dgraph.io/mutations/#upsert-block.

```js
const query = `
  query {
      user as var(func: eq(email, "wrong_email@dgraph.io"))
  }`

const mu = new dgraph.Mutation();
mu.setSetNquads(`uid(user) <email> "correct_email@dgraph.io" .`);

const req = new dgraph.Request();
req.setQuery(query);
req.setMutationsList([mu]);
req.setCommitNow(true);

// Upsert: If wrong_email found, update the existing data
// or else perform a new mutation.
await dgraphClient.newTxn().doRequest(req);
```

### Running a Conditional Upsert

The upsert block allows specifying a conditional mutation block using an `@if` directive. The mutation is executed
only when the specified condition is true. If the condition is false, the mutation is silently ignored.

See more about Conditional Upsert [Here](https://docs.dgraph.io/mutations/#conditional-upsert).

```js
const query = `
  query {
      user as var(func: eq(email, "wrong_email@dgraph.io"))
  }`

const mu = new dgraph.Mutation();
mu.setSetNquads(`uid(user) <email> "correct_email@dgraph.io" .`);
mu.setCond(`@if(eq(len(user), 1))`);

const req = new dgraph.Request();
req.setQuery(query);
req.addMutations(mu);
req.setCommitNow(true);

await dgraphClient.newTxn().doRequest(req);
```

### Committing a Transaction

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

### Cleanup Resources

To cleanup resources, you have to call `DgraphClientStub#close()` individually for
all the instances of `DgraphClientStub`.

```js
const SERVER_ADDR = "localhost:9080";
const SERVER_CREDENTIALS = grpc.credentials.createInsecure();

// Create instances of DgraphClientStub.
const stub1 = new dgraph.DgraphClientStub(SERVER_ADDR, SERVER_CREDENTIALS);
const stub2 = new dgraph.DgraphClientStub(SERVER_ADDR, SERVER_CREDENTIALS);

// Create an instance of DgraphClient.
const dgraphClient = new dgraph.DgraphClient(stub1, stub2);

// ...
// Use dgraphClient
// ...

// Cleanup resources by closing all client stubs.
stub1.close();
stub2.close();
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

### Setting Metadata Headers

Metadata headers such as authentication tokens can be set through the context of gRPC methods. Below is an example of how to set a header named "auth-token".

```js
// The following piece of code shows how one can set metadata with
// auth-token, to allow Alter operation, if the server requires it.

var meta = new grpc.Metadata();
meta.add('auth-token', 'mySuperSecret');

await dgraphClient.alter(op, meta);
```
