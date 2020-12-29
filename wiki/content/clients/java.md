+++
date = "2017-03-20T22:25:17+11:00"
title = "Java"
weight = 4
[menu.main]
    parent = "clients"
+++

A minimal implementation for a Dgraph client for Java 1.8 and above, using [gRPC](https://grpc.io/).
This client follows the [Dgraph Go client]({{< relref "go.md" >}}) closely.

{{% notice "tip" %}}
The official Java client [can be found here](https://github.com/dgraph-io/dgraph4j). 
Follow the [install instructions](https://github.com/dgraph-io/dgraph4j#download) to get it up and running.
{{% /notice %}}

## Supported Versions

More details on the supported versions can be found at [this link](https://github.com/dgraph-io/dgraph4j#supported-versions).

## Quickstart
Build and run the [DgraphJavaSample](https://github.com/dgraph-io/dgraph4j/tree/master/samples/DgraphJavaSample) project in the `samples` folder, which
contains an end-to-end example of using the Dgraph Java client. Follow the
instructions in the [README](https://github.com/dgraph-io/dgraph4j/tree/master/samples/DgraphJavaSample/README.md) of that project.

## Intro
This library supports two styles of clients, the synchronous client `DgraphClient` and
the async client `DgraphAsyncClient`.
A `DgraphClient` or `DgraphAsyncClient` can be initialised by passing it
a list of `DgraphBlockingStub` clients. The `anyClient()` API can randomly pick a stub, which can
then be used for GRPC operations. In the next section, we will explain how to create a
synchronous client and use it to mutate or query dgraph. For the async client, more details can
be found in the [Using the Asynchronous Client](#using-the-asynchronous-client) section.

## Using the Synchronous Client

{{% notice "tip" %}}
You can find a [DgraphJavaSample](https://github.com/dgraph-io/dgraph4j/tree/master/samples/DgraphJavaSample) project, 
which contains an end-to-end working example of how to use the Java client.
{{% /notice %}}

### Creating a Client

The following code snippet shows how to create a synchronous client using three connections.

```java
ManagedChannel channel1 = ManagedChannelBuilder
    .forAddress("localhost", 9080)
    .usePlaintext().build();
DgraphStub stub1 = DgraphGrpc.newStub(channel1);

ManagedChannel channel2 = ManagedChannelBuilder
    .forAddress("localhost", 9082)
    .usePlaintext().build();
DgraphStub stub2 = DgraphGrpc.newStub(channel2);

ManagedChannel channel3 = ManagedChannelBuilder
    .forAddress("localhost", 9083)
    .usePlaintext().build();
DgraphStub stub3 = DgraphGrpc.newStub(channel3);

DgraphClient dgraphClient = new DgraphClient(stub1, stub2, stub3);
```

### Creating a Client for Slash Graphql Endpoint

If you want to connect to Dgraph running on your [Slash GraphQL](https://slash.dgraph.io) instance, then all you need is the URL of your Slash GraphQL endpoint and the API key. You can get a client using them as follows :

```java
DgraphStub stub = DgraphClient.clientStubFromSlashEndpoint("https://your-slash-instance.cloud.dgraph.io/graphql", "your-api-key");
DgraphClient dgraphClient = new DgraphClient(stub);
```

### Creating a Secure Client using TLS

To setup a client using TLS, you could use the following code snippet. The server needs to be
setup using the instructions provided [here](https://docs.dgraph.io/deploy/#tls-configuration).

If you are doing client verification, you need to convert the client key from PKCS#1 format to
PKCS#8 format. By default, grpc doesn't support reading PKCS#1 format keys. To convert the
format, you could use the `openssl` tool.

First, let's install the `openssl` tool:
```sh
apt install openssl
```

Now, use the following command to convert the key:
```sh
openssl pkcs8 -in client.name.key -topk8 -nocrypt -out client.name.java.key
```

Now, you can use the following code snippet to connect to Alpha over TLS:

```java
SslContextBuilder builder = GrpcSslContexts.forClient();
builder.trustManager(new File("<path to ca.crt>"));
// Skip the next line if you are not performing client verification.
builder.keyManager(new File("<path to client.name.crt>"), new File("<path to client.name.java.key>"));
SslContext sslContext = builder.build();

ManagedChannel channel = NettyChannelBuilder.forAddress("localhost", 9080)
    .sslContext(sslContext)
    .build();
DgraphGrpc.DgraphStub stub = DgraphGrpc.newStub(channel);
DgraphClient dgraphClient = new DgraphClient(stub);
```

### Check Dgraph version

Checking the version of the Dgraph server this client is interacting with is as easy as:
```java
Version v = dgraphClient.checkVersion();
System.out.println(v.getTag());
```
Checking the version, before doing anything else can be used as a test to find out if the client
is able to communicate with the Dgraph server. This will also help reduce the latency of the first
query/mutation which results from some dynamic library loading and linking that happens in JVM
(see [this issue](https://github.com/dgraph-io/dgraph4j/issues/108) for more details).

### Login Using ACL

```java
dgraphClient.login(USER_ID, USER_PASSWORD);
```

### Altering the Database

To set the schema, create an `Operation` object, set the schema and pass it to
`DgraphClient#alter` method.

```java
String schema = "name: string @index(exact) .";
Operation operation = Operation.newBuilder().setSchema(schema).build();
dgraphClient.alter(operation);
```

Starting Dgraph version 20.03.0, indexes can be computed in the background.
You can call the function `setRunInBackground(true)` as shown below before
calling `alter`. You can find more details
[here](https://docs.dgraph.io/master/query-language/#indexes-in-background).

```java
String schema = "name: string @index(exact) .";
Operation operation = Operation.newBuilder()
        .setSchema(schema)
        .setRunInBackground(true)
        .build();
dgraphClient.alter(operation);
```

`Operation` contains other fields as well, including drop predicate and
drop all. Drop all is useful if you wish to discard all the data, and start from
a clean slate, without bringing the instance down.

```java
// Drop all data including schema from the dgraph instance. This is useful
// for small examples such as this, since it puts dgraph into a clean
// state.
dgraphClient.alter(Operation.newBuilder().setDropAll(true).build());
```

### Creating a Transaction

There are two types of transactions in dgraph, i.e. the read-only transactions that only include
queries and the transactions that change data in dgraph with mutate operations. Both the
synchronous client `DgraphClient` and the async client `DgraphAsyncClient` support the two types
of transactions by providing the `newTransaction` and the `newReadOnlyTransaction` APIs. Creating
 a transaction is a local operation and incurs no network overhead.

In most of the cases, the normal read-write transactions is used, which can have any
number of query or mutate operations. However, if a transaction only has queries, you might
benefit from a read-only transaction, which can share the same read timestamp across multiple
such read-only transactions and can result in lower latencies.

For normal read-write transactions, it is a good practise to call `Transaction#discard()` in a
`finally` block after running the transaction. Calling `Transaction#discard()` after
`Transaction#commit()` is a no-op and you can call `discard()` multiple times with no additional
side-effects.

```java
Transaction txn = dgraphClient.newTransaction();
try {
    // Do something here
    // ...
} finally {
    txn.discard();
}
```

For read-only transactions, there is no need to call `Transaction.discard`, which is equivalent
to a no-op.

```java
Transaction readOnlyTxn = dgraphClient.newReadOnlyTransaction();
```

Read-only transactions can be set as best-effort. Best-effort queries relax the requirement of
linearizable reads. This is useful when running queries that do not require a result from the latest
timestamp.

```java
Transaction bestEffortTxn = dgraphClient.newReadOnlyTransaction()
    .setBestEffort(true);
```

### Running a Mutation
`Transaction#mutate` runs a mutation. It takes in a `Mutation` object,
which provides two main ways to set data: JSON and RDF N-Quad. You can choose
whichever way is convenient.

We're going to use JSON. First we define a `Person` class to represent a person.
This data will be serialized into JSON.

```java
class Person {
    String name
    Person() {}
}
```

Next, we initialise a `Person` object, serialize it and use it in `Mutation` object.

```java
// Create data
Person person = new Person();
person.name = "Alice";

// Serialize it
Gson gson = new Gson();
String json = gson.toJson(person);
// Run mutation
Mutation mu = Mutation.newBuilder()
    .setSetJson(ByteString.copyFromUtf8(json.toString()))
    .build();
txn.mutate(mu);
```

Sometimes, you only want to commit mutation, without querying anything further.
In such cases, you can use a `CommitNow` field in `Mutation` object to
indicate that the mutation must be immediately committed.

Mutation can be run using the `doRequest` function as well.

```java
Request request = Request.newBuilder()
    .addMutations(mu)
    .build();
txn.doRequest(request);
```

### Committing a Transaction
A transaction can be committed using the `Transaction#commit()` method. If your transaction
consisted solely of calls to `Transaction#query()`, and no calls to `Transaction#mutate()`,
then calling `Transaction#commit()` is not necessary.

An error will be returned if other transactions running concurrently modify the same data that was
modified in this transaction. It is up to the user to retry transactions when they fail.

```java
Transaction txn = dgraphClient.newTransaction();

try {
    // …
    // Perform any number of queries and mutations
    // …
    // and finally …
    txn.commit()
} catch (TxnConflictException ex) {
    // Retry or handle exception.
} finally {
    // Clean up. Calling this after txn.commit() is a no-op
    // and hence safe.
    txn.discard();
}
```

### Running a Query
You can run a query by calling `Transaction#query()`. You will need to pass in a GraphQL+-
query string, and a map (optional, could be empty) of any variables that you might want to
set in the query.

The response would contain a `JSON` field, which has the JSON encoded result. You will need
to decode it before you can do anything useful with it.

Let’s run the following query:

```
query all($a: string) {
  all(func: eq(name, $a)) {
            name
  }
}
```

First we must create a `People` class that will help us deserialize the JSON result:

```java
class People {
    List<Person> all;
    People() {}
}
```

Then we run the query, deserialize the result and print it out:

```java
// Query
String query =
"query all($a: string){\n" +
"  all(func: eq(name, $a)) {\n" +
"    name\n" +
"  }\n" +
"}\n";

Map<String, String> vars = Collections.singletonMap("$a", "Alice");
Response response = dgraphClient.newReadOnlyTransaction().queryWithVars(query, vars);

// Deserialize
People ppl = gson.fromJson(response.getJson().toStringUtf8(), People.class);

// Print results
System.out.printf("people found: %d\n", ppl.all.size());
ppl.all.forEach(person -> System.out.println(person.name));
```
This should print:

```
people found: 1
Alice
```

You can also use `doRequest` function to run the query.

```java
Request request = Request.newBuilder()
    .setQuery(query)
    .build();
txn.doRequest(request);
```

### Running a Query with RDF response

You can get query results as an RDF response by calling either `queryRDF()` or `queryRDFWithVars()`. 
The response contains the `getRdf()` method, which will provide the RDF encoded output.

**Note**: If you are querying for `uid` values only, use a JSON format response

```java
// Query
String query = "query me($a: string) { me(func: eq(name, $a)) { name }}";
Map<String, String> vars = Collections.singletonMap("$a", "Alice");
Response response =
    dgraphAsyncClient.newReadOnlyTransaction().queryRDFWithVars(query, vars).join();

// Print results
System.out.println(response.getRdf().toStringUtf8());
```

This should print (assuming Alice's `uid` is `0x2`):

```
<0x2> <name> "Alice" .
```

### Running an Upsert: Query + Mutation

The `txn.doRequest` function allows you to run upserts consisting of one query and
one mutation. Variables can be defined in the query and used in the mutation.
You could also use `txn.doRequest` to perform a query followed by a mutation.

To know more about upsert, we highly recommend going through the docs at
https://docs.dgraph.io/mutations/#upsert-block.

```java
String query = "query {\n" +
  "user as var(func: eq(email, \"wrong_email@dgraph.io\"))\n" +
  "}\n";
Mutation mu = Mutation.newBuilder()
    .setSetNquads(ByteString.copyFromUtf8("uid(user) <email> \"correct_email@dgraph.io\" ."))
    .build();
Request request = Request.newBuilder()
    .setQuery(query)
    .addMutations(mu)
    .setCommitNow(true)
    .build();
txn.doRequest(request);
```

### Running a Conditional Upsert

The upsert block also allows specifying a conditional mutation block using an `@if` directive. The mutation is executed
only when the specified condition is true. If the condition is false, the mutation is silently ignored.

See more about Conditional Upsert [Here](https://docs.dgraph.io/mutations/#conditional-upsert).

```java
String query = "query {\n" +
    "user as var(func: eq(email, \"wrong_email@dgraph.io\"))\n" +
    "}\n";
Mutation mu = Mutation.newBuilder()
    .setSetNquads(ByteString.copyFromUtf8("uid(user) <email> \"correct_email@dgraph.io\" ."))
    .setCond("@if(eq(len(user), 1))")
    .build();
Request request = Request.newBuilder()
    .setQuery(query)
    .addMutations(mu)
    .setCommitNow(true)
    .build();
txn.doRequest(request);
```

### Setting Deadlines
It is recommended that you always set a deadline for each client call, after
which the client terminates. This is in line with the recommendation for any gRPC client.
Read [this forum post][deadline-post] for more details.

```java
channel = ManagedChannelBuilder.forAddress("localhost", 9080).usePlaintext(true).build();
DgraphGrpc.DgraphStub stub = DgraphGrpc.newStub(channel);
ClientInterceptor timeoutInterceptor = new ClientInterceptor(){
    @Override
    public <ReqT, RespT> ClientCall<ReqT, RespT> interceptCall(
            MethodDescriptor<ReqT, RespT> method, CallOptions callOptions, Channel next) {
        return next.newCall(method, callOptions.withDeadlineAfter(500, TimeUnit.MILLISECONDS));
    }
};
stub.withInterceptors(timeoutInterceptor);
DgraphClient dgraphClient = new DgraphClient(stub);
```

[deadline-post]: https://discuss.dgraph.io/t/dgraph-java-client-setting-deadlines-per-call/3056

### Setting Metadata Headers
Certain headers such as authentication tokens need to be set globally for all subsequent calls.
Below is an example of setting a header with the name "auth-token":
```java
// create the stub first
ManagedChannel channel =
ManagedChannelBuilder.forAddress(TEST_HOSTNAME, TEST_PORT).usePlaintext(true).build();
DgraphStub stub = DgraphGrpc.newStub(channel);

// use MetadataUtils to augment the stub with headers
Metadata metadata = new Metadata();
metadata.put(
        Metadata.Key.of("auth-token", Metadata.ASCII_STRING_MARSHALLER), "the-auth-token-value");
stub = MetadataUtils.attachHeaders(stub, metadata);

// create the DgraphClient wrapper around the stub
DgraphClient dgraphClient = new DgraphClient(stub);

// trigger a RPC call using the DgraphClient
dgraphClient.alter(Operation.newBuilder().setDropAll(true).build());
```
### Helper Methods

#### Delete multiple edges
The example below uses the helper method `Helpers#deleteEdges` to delete
multiple edges corresponding to predicates on a node with the given uid.
The helper method takes an existing mutation, and returns a new mutation
with the deletions applied.

```java
Mutation mu = Mutation.newBuilder().build()
mu = Helpers.deleteEdges(mu, uid, "friends", "loc");
dgraphClient.newTransaction().mutate(mu);
```

### Closing the DB Connection

To disconnect from Dgraph, call `ManagedChannel#shutdown` on the gRPC
channel object created when [creating a Dgraph
client](#creating-a-client).

```
channel.shutdown();
```

## Using the Asynchronous Client
Dgraph Client for Java also bundles an asynchronous API, which can be used by
instantiating the `DgraphAsyncClient` class. The usage is almost exactly the
same as the `DgraphClient` (show in previous section) class. The main
differences is that the `DgraphAsyncClient#newTransacation()` returns an
`AsyncTransaction` class. The API for `AsyncTransaction` is exactly
`Transaction`. The only difference is that instead of returning the results
directly, it returns immediately with a corresponding `CompletableFuture<T>`
object. This object represents the computation which runs asynchronously to
yield the result in the future. Read more about `CompletableFuture<T>` in the
[Java 8 documentation][futuredocs].

[futuredocs]: https://docs.oracle.com/javase/8/docs/api/java/util/concurrent/CompletableFuture.html

Here is the asynchronous version of the code above, which runs a query.

```java
// Query
String query = 
"query all($a: string){\n" +
"  all(func: eq(name, $a)) {\n" +
"    name\n" +
 "}\n" +
"}\n";

Map<String, String> vars = Collections.singletonMap("$a", "Alice");

AsyncTransaction txn = dgraphAsyncClient.newTransaction();
txn.query(query).thenAccept(response -> {
    // Deserialize
    People ppl = gson.fromJson(res.getJson().toStringUtf8(), People.class);

    // Print results
    System.out.printf("people found: %d\n", ppl.all.size());
    ppl.all.forEach(person -> System.out.println(person.name));
});
```
## Checking the request latency
If you would like to see the latency for either a mutation or
query request, the latency field in the returned result can be helpful. Here is an example to log
 the latency of a query request:
```java
Response resp = txn.query(query);
Latency latency = resp.getLatency();
logger.info("parsing latency:" + latency.getParsingNs());
logger.info("processing latency:" + latency.getProcessingNs());
logger.info("encoding latency:" + latency.getEncodingNs());
```
Similarly you can get the latency of a mutation request:
```java
Assigned assignedIds = dgraphClient.newTransaction().mutate(mu);
Latency latency = assignedIds.getLatency();
```
