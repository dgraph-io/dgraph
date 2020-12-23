+++
date = "2017-03-20T22:25:17+11:00"
title = "C#"
weight = 3
[menu.main]
    parent = "clients"
+++

The official C# client [can be found here](https://github.com/dgraph-io/dgraph.net).
Follow the instructions in the [README](https://github.com/dgraph-io/dgraph.net#readme) to get
it up and running.

More details on the supported versions can be found at
[this link](https://github.com/dgraph-io/dgraph.net#supported-versions).

# Dgraph.net ![Nuget](https://img.shields.io/nuget/v/dgraph) [![Build Status](https://teamcity.dgraph.io/guestAuth/app/rest/builds/buildType:(id:DgraphNet_Build)/statusIcon.svg)](https://teamcity.dgraph.io/viewLog.html?buildTypeId=DgraphNet_Build&buildId=lastFinished&guest=1)

This client follows the [Dgraph Go client][goclient] closely.

[goclient]: https://github.com/dgraph-io/dgo

Before using this client, we highly recommend that you go through [docs.dgraph.io],
and understand how to run and work with Dgraph.

**Use [Discuss Issues](https://discuss.dgraph.io/tags/c/issues/35/dgraphnet) for reporting issues about this repository.**

[docs.dgraph.io]:https://docs.dgraph.io

## Table of contents

  - [Install](#install)
  - [Supported Versions](#supported-versions)
  - [Using a Client](#using-a-client)
    - [Creating a Client](#creating-a-client)
    - [Altering the Database](#altering-the-database)
    - [Creating a Transaction](#creating-a-transaction)
    - [Running a Mutation](#running-a-mutation)
    - [Running a Query](#running-a-query)
    - [Running an Upsert: Query + Mutation](#running-an-upsert-query--mutation)
    - [Committing a Transaction](#committing-a-transaction)
    - [Cleanup Resources](#cleanup-resources)

## Install

Install using nuget:

```sh
dotnet add package Dgraph
```

## Supported Versions

Each release of this client will support the equivalent Dgraph release. For example, 2020.03.XX will support any Dgraph instances with version 2020.03.XX. 


## Using a Client

### Creating a Client

Make a new client by passing in one or more GRPC channels pointing to alphas.

```c#
var client = new DgraphClient(new Channel("127.0.0.1:9080", ChannelCredentials.Insecure));
```


### Altering the Database

To set the schema, pass the schema into the `DgraphClient.Alter` function, as seen below:

```c#
var schema = "`name: string @index(exact) .";
var result = client.Alter(new Operation{ Schema = schema });
```

The returned result object is based on the FluentResults library. You can check the status using `result.isSuccess` or `result.isFailed`. More information on the result object can be found [here](https://github.com/altmann/FluentResults).


### Creating a Transaction

To create a transaction, call `DgraphClient.NewTransaction` method, which returns a
new `Transaction` object. This operation incurs no network overhead.

It is good practice to call to wrap the `Transaction` in a `using` block, so that the `Transaction.Dispose` function is called after running
the transaction. 

```c#
using(var transaction = client.NewTransaction()) {
    ...
}
```

You can also create Read-Only transactions. Read-Only transactions only allow querying, and can be created using `DgraphClient.NewReadOnlyTransaction`.


### Running a Mutation

`Transaction.Mutate(RequestBuilder)` runs a mutation. It takes in a json mutation string.

We define a person object to represent a person and serialize it to a json mutation string. In this example, we are using the [JSON.NET](https://www.newtonsoft.com/json) library, but you can use any JSON serialization library you prefer.

```c#
using(var txn = client.NewTransaction()) {
    var alice = new Person{ Name = "Alice" };
    var json = JsonConvert.SerializeObject(alice);
    
    var transactionResult = await txn.Mutate(new RequestBuilder().WithMutations(new MutationBuilder{ SetJson = json }));
}
```

You can also set mutations using RDF format, if you so prefer, as seen below:

```c#
var mutation = "_:alice <name> \"Alice\"";
var transactionResult = await txn.Mutate(new RequestBuilder().WithMutations(new MutationBuilder{ SetNquads = mutation }));
```

Check out the example in `source/Dgraph.tests.e2e/TransactionTest.cs`.

### Running a Query

You can run a query by calling `Transaction.Query(string)`. You will need to pass in a
GraphQL+- query string. If you want to pass an additional map of any variables that
you might want to set in the query, call `Transaction.QueryWithVars(string, Dictionary<string,string>)` with
the variables dictionary as the second argument.

The response would contain the response string.

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

```c#
// Run query.
var query = @"query all($a: string) {
  all(func: eq(name, $a))
  {
    name
  }
}";

var vars = new Dictionary<string,string> { { $a: "Alice" } };
var res = await dgraphClient.NewReadOnlyTransaction().QueryWithVars(query, vars);

// Print results.
Console.Write(res.Value.Json);
```

### Running an Upsert: Query + Mutation

The `Transaction.Mutate` function allows you to run upserts consisting of one query and one mutation. 

To know more about upsert, we highly recommend going through the docs at https://docs.dgraph.io/mutations/#upsert-block.

```c#
var query = @"
  query {
    user as var(func: eq(email, \"wrong_email@dgraph.io\"))
  }";

var mutation = new MutationBuilder{ SetNquads = "`uid(user) <email> \"correct_email@dgraph.io\" ." };

var request = new RequestBuilder{ Query = query, CommitNow = true }.withMutation(mutation);

// Upsert: If wrong_email found, update the existing data
// or else perform a new mutation.
await txn.Mutate(request);
```

### Committing a Transaction

A transaction can be committed using the `Transaction.Commit` method. If your transaction
consisted solely of calls to `Transaction.Query` or `Transaction.QueryWithVars`, and no calls to
`Transaction.Mutate`, then calling `Transaction.Commit` is not necessary.

An error will be returned if other transactions running concurrently modify the same
data that was modified in this transaction. It is up to the user to retry
transactions when they fail.

```c#
using(var txn = client.NewTransaction()) {
    var result = txn.Commit();
}
```
