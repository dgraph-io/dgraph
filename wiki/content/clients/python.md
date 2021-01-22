+++
date = "2017-03-20T22:25:17+11:00"
title = "Python"
weight = 6
[menu.main]
    parent = "clients"
+++

Official Dgraph client implementation for Python (Python >= v2.7 and >= v3.5), using [gRPC](https://grpc.io/).
This client follows the [Dgraph Go client]({{< relref "go.md" >}}) closely.

{{% notice "tip" %}}
The official Python client [can be found here](https://github.com/dgraph-io/pydgraph). 
Follow the [install instructions](https://github.com/dgraph-io/pydgraph#install) to get it
up and running.
{{% /notice %}}

## Supported Versions

More details on the supported versions can be found at [this link](https://github.com/dgraph-io/pydgraph#supported-versions).

## Using a client

{{% notice "tip" %}}
You can get a [simple example](https://github.com/dgraph-io/pydgraph/tree/master/examples/simple)
project, which contains an end-to-end working example of how to use the Python client.
{{% /notice %}}

### Creating a Client

You can initialize a `DgraphClient` object by passing it a list of
`DgraphClientStub` clients as variadic arguments. Connecting to multiple Dgraph
servers in the same cluster allows for better distribution of workload.

The following code snippet shows just one connection.

```python
import pydgraph

client_stub = pydgraph.DgraphClientStub('localhost:9080')
client = pydgraph.DgraphClient(client_stub)
```

### Altering the Database

To set the schema, create an `Operation` object, set the schema and pass it to
`DgraphClient#alter(Operation)` method.

```python
schema = 'name: string @index(exact) .'
op = pydgraph.Operation(schema=schema)
client.alter(op)
```

Starting with Dgraph version 20.03.0, indexes can be computed in the background.
You can set the `run_in_background` field of `pydgraph.Operation` to `True`
before passing it to the `Alter` function. You can find more details
[here](https://docs.dgraph.io/master/query-language/#indexes-in-background).

```python
schema = 'name: string @index(exact) .'
op = pydgraph.Operation(schema=schema, run_in_background=True)
client.alter(op)
```

`Operation` contains other fields as well, including the `drop` predicate and `drop all`.
Drop all is useful if you wish to discard all the data, and start with a clean
slate, without bringing the instance down.

```python
# Drop all data including schema from the Dgraph instance. This is a useful
# for small examples such as this since it puts Dgraph into a clean state.
op = pydgraph.Operation(drop_all=True)
client.alter(op)
```

### Creating a Transaction

To create a transaction, call the `DgraphClient#txn()` method, which returns a
new `Txn` object. This operation incurs no network overhead.

It is good practice to call `Txn#discard()` in a `finally` block after running
the transaction. Calling `Txn#discard()` after `Txn#commit()` is a no-op
and you can call `Txn#discard()` multiple times with no additional side-effects.

```python
txn = client.txn()
try:
  # Do something here
  # ...
finally:
  txn.discard()
  # ...
```

To create a read-only transaction, call `DgraphClient#txn(read_only=True)`.
Read-only transactions are ideal for transactions which only involve queries.
Mutations and commits are not allowed.

```python
txn = client.txn(read_only=True)
try:
  # Do some queries here
  # ...
finally:
  txn.discard()
  # ...
```

To create a read-only transaction that executes best-effort queries, call
`DgraphClient#txn(read_only=True, best_effort=True)`. Best-effort queries are
faster than normal queries because they bypass the normal consensus protocol.
For this same reason, best-effort queries cannot guarantee to return the latest
data. Best-effort queries are only supported by read-only transactions.

### Running a Mutation

`Txn#mutate(mu=Mutation)` runs a mutation. It takes in a `Mutation` object,
which provides two main ways to set data: JSON and RDF N-Quad. You can choose
whichever way is convenient.

`Txn#mutate()` provides convenience keyword arguments `set_obj` and `del_obj`
for setting JSON values and `set_nquads` and `del_nquads` for setting N-Quad
values. See examples below for usage.

We define a person object to represent a person and use it in a transaction.

```python
# Create data.
p = {
    'name': 'Alice',
}

# Run mutation.
txn.mutate(set_obj=p)

# If you want to use a mutation object, use this instead:
# mu = pydgraph.Mutation(set_json=json.dumps(p).encode('utf8'))
# txn.mutate(mu)

# If you want to use N-Quads, use this instead:
# txn.mutate(set_nquads='_:alice <name> "Alice" .')
```

```python
# Delete data.

query = """query all($a: string)
 {
   all(func: eq(name, $a))
    {
      uid
    }
  }"""

variables = {'$a': 'Bob'}

res = txn.query(query, variables=variables)
ppl = json.loads(res.json)

# For a mutation to delete a node, use this:
txn.mutate(del_obj=person)
```

For a complete example with multiple fields and relationships, look at the
[simple project][simple] in the `examples` folder.

Sometimes, you only want to commit a mutation, without querying anything further.
In such cases, you can set the keyword argument `commit_now=True` to indicate
that the mutation must be immediately committed.

A mutation can be executed using `txn.do_request` as well.

```python
mutation = txn.create_mutation(set_nquads='_:alice <name> "Alice" .')
request = txn.create_request(mutations=[mutation], commit_now=True)
txn.do_request(request)
```

### Committing a Transaction

A transaction can be committed using the `Txn#commit()` method. If your transaction
consist solely of `Txn#query` or `Txn#queryWithVars` calls, and no calls to
`Txn#mutate`, then calling `Txn#commit()` is not necessary.

An error is raised if another transaction(s) modify the same data concurrently that was
modified in the current transaction. It is up to the user to retry transactions
when they fail.

```python
txn = client.txn()
try:
  # ...
  # Perform any number of queries and mutations
  # ...
  # and finally...
  txn.commit()
except pydgraph.AbortedError:
  # Retry or handle exception.
finally:
  # Clean up. Calling this after txn.commit() is a no-op
  # and hence safe.
  txn.discard()
```

### Running a Query

You can run a query by calling `Txn#query(string)`. You will need to pass in a
[DQL](https://dgraph.io/docs/query-language/) query string. If you want to pass an additional dictionary of any
variables that you might want to set in the query, call
`Txn#query(string, variables=d)` with the variables dictionary `d`.

The query response contains the `json` field, which returns the JSON response.

Let’s run a query with a variable `$a`, deserialize the result from JSON and
print it out:

```python
# Run query.
query = """query all($a: string) {
  all(func: eq(name, $a))
  {
    name
  }
}"""
variables = {'$a': 'Alice'}

res = txn.query(query, variables=variables)

# If not doing a mutation in the same transaction, simply use:
# res = client.txn(read_only=True).query(query, variables=variables)

ppl = json.loads(res.json)

# Print results.
print('Number of people named "Alice": {}'.format(len(ppl['all'])))
for person in ppl['all']:
  print(person)
```

This should print:

```console
Number of people named "Alice": 1
Alice
```

You can also use `txn.do_request` function to run the query.

```python
request = txn.create_request(query=query)
txn.do_request(request)
```

### Running an Upsert: Query + Mutation

The `txn.do_request` function allows you to use upsert blocks. An upsert block
contains one query block and one or more mutation blocks, so it lets you perform
queries and mutations in a single request. Variables defined in the query block
can be used in the mutation blocks using the `uid` and `val` functions
implemented by DQL.

To learn more about upsert blocks, see the
[Upsert Block documentation](https://dgraph.io/docs/mutations/upsert-block/).

```python
query = """{
  u as var(func: eq(name, "Alice"))
}"""
nquad = """
  uid(u) <name> "Alice" .
  uid(u) <age> "25" .
"""
mutation = txn.create_mutation(set_nquads=nquad)
request = txn.create_request(query=query, mutations=[mutation], commit_now=True)
txn.do_request(request)
```

### Running a Conditional Upsert

The upsert block also allows specifying a conditional mutation block using an `@if` directive. The mutation is executed
only when the specified condition is true. If the condition is false, the mutation is silently ignored.

See more about Conditional Upserts [here](https://docs.dgraph.io/mutations/#conditional-upsert).

```python
query = """
  {
    user as var(func: eq(email, "wrong_email@dgraph.io"))
  }
"""
cond = "@if(eq(len(user), 1))"
nquads = """
  uid(user) <email> "correct_email@dgraph.io" .
"""
mutation = txn.create_mutation(cond=cond, set_nquads=nquads)
request = txn.create_request(mutations=[mutation], query=query, commit_now=True)
txn.do_request(request)
```

### Cleaning Up Resources

To clean up resources, you have to call `DgraphClientStub#close()` individually for
all the instances of `DgraphClientStub`.

```python
SERVER_ADDR = "localhost:9080"

# Create instances of DgraphClientStub.
stub1 = pydgraph.DgraphClientStub(SERVER_ADDR)
stub2 = pydgraph.DgraphClientStub(SERVER_ADDR)

# Create an instance of DgraphClient.
client = pydgraph.DgraphClient(stub1, stub2)

# ...
# Use client
# ...

# Clean up resources by closing all client stubs.
stub1.close()
stub2.close()
```

### Setting Metadata Headers

Metadata headers such as authentication tokens can be set through the metadata of gRPC methods. Below is an example of how to set a header named "auth-token".

```python
# The following piece of code shows how one can set metadata with
# auth-token, to allow Alter operation, if the server requires it.
# metadata is a list of arbitrary key-value pairs.
metadata = [("auth-token", "the-auth-token-value")]
dg.alter(op, metadata=metadata)
```

### Setting a timeout

A timeout value representing the number of seconds can be passed to the `login`,
`alter`, `query`, and `mutate` methods using the `timeout` keyword argument.

For example, the following alters the schema with a timeout of ten seconds:
`dg.alter(op, timeout=10)`

### Passing credentials

A `CallCredentials` object can be passed to the `login`, `alter`, `query`, and
`mutate` methods using the `credentials` keyword argument.

### Authenticating to a reverse TLS proxy

If the Dgraph instance is behind a reverse TLS proxy, credentials can also be
passed through the methods available in the gRPC library. Note that in this case
every request will need to include the credentials. In the example below, we are
trying to add authentication to a proxy that requires an API key. This value is
expected to be included in the metadata using the key "authorization".

```python
creds = grpc.ssl_channel_credentials()
call_credentials = grpc.metadata_call_credentials(
    lambda context, callback: callback((("authorization", "<api-key>"),), None))
composite_credentials = grpc.composite_channel_credentials(creds, call_credentials)
client_stub = pydgraph.DgraphClientStub(
    '{host}:{port}'.format(host=GRPC_HOST, port=GRPC_PORT), composite_credentials)
client = pydgraph.DgraphClient(client_stub)
```

### Async methods

The `alter` method in the client has an asynchronous version called
`async_alter`. The async methods return a future. You can directly call the
`result` method on the future. However. The DgraphClient class provides a static
method `handle_alter_future` to handle any possible exception.

```python
alter_future = self.client.async_alter(pydgraph.Operation(
	schema="name: string @index(term) ."))
response = pydgraph.DgraphClient.handle_alter_future(alter_future)
```

The `query` and `mutate` methods int the `Txn` class also have async versions
called `async_query` and `async_mutation` respectively. These functions work
just like `async_alter`.

You can use the `handle_query_future` and `handle_mutate_future` static methods
in the `Txn` class to retrieve the result. A short example is given below:

```python
txn = client.txn()
query = "query body here"
future = txn.async_query()
response = pydgraph.Txn.handle_query_future(future)
```

A working example can be found in the `test_asycn.py` test file.

Keep in mind that due to the nature of async calls, the async functions cannot
retry the request if the login is invalid. You will have to check for this error
and retry the login (with the function `retry_login` in both the `Txn` and
`Client` classes). A short example is given below:

```python
client = DgraphClient(client_stubs) # client_stubs is a list of gRPC stubs.
alter_future = client.async_alter()
try:
    response = alter_future.result()
except Exception as e:
	# You can use this function in the util package to check for JWT
    # expired errors.
    if pydgraph.util.is_jwt_expired(e):
        # retry your request here.
```
