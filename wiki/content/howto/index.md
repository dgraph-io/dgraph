+++
date = "2017-03-20T19:35:35+11:00"
title = "How To Guides"
+++

## Retrieving Debug Information

Each Dgraph data node exposes profile over `/debug/pprof` endpoint and metrics over `/debug/vars` endpoint. Each Dgraph data node has it's own profiling and metrics information. Below is a list of debugging information exposed by Dgraph and the corresponding commands to retrieve them.

### Metrics Information

If you are collecting these metrics from outside the Dgraph instance you need to pass `--expose_trace=true` flag, otherwise there metrics can be collected by connecting to the instance over localhost.

```
curl http://<IP>:<HTTP_PORT>/debug/vars
```

Metrics can also be retrieved in the Prometheus format at `/debug/prometheus_metrics`. See the [Metrics]({{< relref "deploy/index.md#metrics" >}}) section for the full list of metrics.

### Profiling Information

Profiling information is available via the `go tool pprof` profiling tool built into Go. The ["Profiling Go programs"](https://blog.golang.org/profiling-go-programs) Go blog post will help you get started with using pprof. Each Dgraph Zero and Dgraph Alpha exposes a debug endpoint at `/debug/pprof/<profile>` via the HTTP port.

```
go tool pprof http://<IP>:<HTTP_PORT>/debug/pprof/heap
#Fetching profile from ...
#Saved Profile in ...
```
The output of the command would show the location where the profile is stored.

In the interactive pprof shell, you can use commands like `top` to get a listing of the top functions in the profile, `web` to get a visual graph of the profile opened in a web browser, or `list` to display a code listing with profiling information overlaid.

#### CPU Profile

```
go tool pprof http://<IP>:<HTTP_PORT>/debug/pprof/profile
```

#### Memory Profile

```
go tool pprof http://<IP>:<HTTP_PORT>/debug/pprof/heap
```

#### Block Profile

Dgraph by default doesn't collect the block profile. Dgraph must be started with `--block=<N>` with N > 1.

```
go tool pprof http://<IP>:<HTTP_PORT>/debug/pprof/block
```

#### Goroutine stack

The HTTP page `/debug/pprof/` is available at the HTTP port of a Dgraph Zero or Dgraph Alpha. From this page a link to the "full goroutine stack dump" is available (e.g., on a Dgraph Alpha this page would be at `http://localhost:8080/debug/pprof/goroutine?debug=2`). Looking at the full goroutine stack can be useful to understand goroutine usage at that moment.

## Using the Debug Tool

{{% notice "note" %}}
To debug a running Dgraph cluster, first copy the postings ("p") directory to
another location. If the Dgraph cluster is not running, then you can use the
same postings directory with the debug tool.
{{% /notice %}}

The `dgraph debug` tool can be used to inspect Dgraph's posting list structure.
You can use the debug tool to inspect the data, schema, and indices of your
Dgraph cluster.

Some scenarios where the debug tool is useful:

- Verify that mutations committed to Dgraph have been persisted to disk.
- Verify that indices are created.
- Inspect the history of a posting list.

### Example Usage

Debug the p directory.

```sh
$ dgraph debug --postings ./p
```

Debug the p directory, not opening in read-only mode. This is typically necessary when the database was not closed properly.

```sh
$ dgraph debug --postings ./p --readonly=false
```

Debug the p directory, only outputing the keys for the predicate `name`.

```sh
$ dgraph debug --postings ./p --readonly=false --pred=name
```

Debug the p directory, looking up a particular key:

```sh
$ dgraph debug --postings ./p --lookup 00000b6465736372697074696f6e020866617374
```

Debug the p directory, inspecting the history of a particular key:

```sh
$ dgraph debug --postings ./p --lookup 00000b6465736372697074696f6e020866617374 --history
```


### Debug Tool Output

Let's go over an example with a Dgraph cluster with the following schema with a term index, full-text index, and two separately committed mutations:

```sh
$ curl localhost:8080/alter -d '
  name: string @index(term) .
  url: string .
  description: string @index(fulltext) .
'
```

```sh
$ curl -H "Content-Type: application/rdf" localhost:8080/mutate?commitNow=true -d '{
  set {
    _:dgraph <name> "Dgraph" .
    _:dgraph <url> "https://github.com/dgraph-io/dgraph" .
    _:dgraph <description> "Fast, Transactional, Distributed Graph Database." .
  }
}'
```

```sh
$ curl -H "Content-Type: application/rdf" localhost:8080/mutate?commitNow=true -d '{
  set {
    _:badger <name> "Badger" .
    _:badger <url> "https://github.com/dgraph-io/badger" .
    _:badger <description> "Embeddable, persistent and fast key-value (KV) database written in pure Go." .
  }
}'
```

After stopping Dgraph, you can run the debug tool to inspect the postings directory:

{{% notice "note" %}}
The debug output can be very large. Typically you would redirect the debug tool to a file first for easier analysis.
{{% /notice %}}

```sh
$ dgraph debug --postings ./p
```

```text
Opening DB: ./p
Min commit: 1. Max commit: 5, w.r.t 18446744073709551615
prefix =
{d} {v.ok} attr: url uid: 1  key: 00000375726c000000000000000001 item: [71, b0100] ts: 3
{d} {v.ok} attr: url uid: 2  key: 00000375726c000000000000000002 item: [71, b0100] ts: 5
{d} {v.ok} attr: name uid: 1  key: 0000046e616d65000000000000000001 item: [43, b0100] ts: 3
{d} {v.ok} attr: name uid: 2  key: 0000046e616d65000000000000000002 item: [43, b0100] ts: 5
{i} {v.ok} attr: name term: [1] badger  key: 0000046e616d650201626164676572 item: [30, b0100] ts: 5
{i} {v.ok} attr: name term: [1] dgraph  key: 0000046e616d650201646772617068 item: [30, b0100] ts: 3
{d} {v.ok} attr: _predicate_ uid: 1  key: 00000b5f7072656469636174655f000000000000000001 item: [104, b0100] ts: 3
{d} {v.ok} attr: _predicate_ uid: 2  key: 00000b5f7072656469636174655f000000000000000002 item: [104, b0100] ts: 5
{d} {v.ok} attr: description uid: 1  key: 00000b6465736372697074696f6e000000000000000001 item: [92, b0100] ts: 3
{d} {v.ok} attr: description uid: 2  key: 00000b6465736372697074696f6e000000000000000002 item: [119, b0100] ts: 5
{i} {v.ok} attr: description term: [8] databas  key: 00000b6465736372697074696f6e020864617461626173 item: [38, b0100] ts: 5
{i} {v.ok} attr: description term: [8] distribut  key: 00000b6465736372697074696f6e0208646973747269627574 item: [40, b0100] ts: 3
{i} {v.ok} attr: description term: [8] embedd  key: 00000b6465736372697074696f6e0208656d62656464 item: [37, b0100] ts: 5
{i} {v.ok} attr: description term: [8] fast  key: 00000b6465736372697074696f6e020866617374 item: [35, b0100] ts: 5
{i} {v.ok} attr: description term: [8] go  key: 00000b6465736372697074696f6e0208676f item: [33, b0100] ts: 5
{i} {v.ok} attr: description term: [8] graph  key: 00000b6465736372697074696f6e02086772617068 item: [36, b0100] ts: 3
{i} {v.ok} attr: description term: [8] kei  key: 00000b6465736372697074696f6e02086b6569 item: [34, b0100] ts: 5
{i} {v.ok} attr: description term: [8] kv  key: 00000b6465736372697074696f6e02086b76 item: [33, b0100] ts: 5
{i} {v.ok} attr: description term: [8] persist  key: 00000b6465736372697074696f6e020870657273697374 item: [38, b0100] ts: 5
{i} {v.ok} attr: description term: [8] pure  key: 00000b6465736372697074696f6e020870757265 item: [35, b0100] ts: 5
{i} {v.ok} attr: description term: [8] transact  key: 00000b6465736372697074696f6e02087472616e73616374 item: [39, b0100] ts: 3
{i} {v.ok} attr: description term: [8] valu  key: 00000b6465736372697074696f6e020876616c75 item: [35, b0100] ts: 5
{i} {v.ok} attr: description term: [8] written  key: 00000b6465736372697074696f6e02087772697474656e item: [38, b0100] ts: 5
{s} {v.ok} attr: url key: 01000375726c item: [13, b0001] ts: 1
{s} {v.ok} attr: name key: 0100046e616d65 item: [23, b0001] ts: 1
{s} {v.ok} attr: _predicate_ key: 01000b5f7072656469636174655f item: [31, b0001] ts: 1
{s} {v.ok} attr: description key: 01000b6465736372697074696f6e item: [41, b0001] ts: 1
{s} {v.ok} attr: dgraph.type key: 01000b6467726170682e74797065 item: [40, b0001] ts: 1
Found 28 keys
```

Each line in the debug output contains a prefix indicating the type of the key: `{d}`: Data key; `{i}`: Index key; `{c}`: Count key; `{r}`: Reverse key; `{s}`: Schema key. In the debug output above, we see data keys, index keys, and schema keys.

Each index key has a corresponding index type. For example, in `attr: name term: [1] dgraph` the `[1]` shows that this is the term index ([0x1][tok_term]); in `attr: description term: [8] fast`, the `[8]` shows that this is the full-text index ([0x8][tok_fulltext]). These IDs match the index IDs in [tok.go][tok].

[tok_term]: https://github.com/dgraph-io/dgraph/blob/ce82aaafba3d9e57cf5ea1aeb9b637193441e1e2/tok/tok.go#L39
[tok_fulltext]: https://github.com/dgraph-io/dgraph/blob/ce82aaafba3d9e57cf5ea1aeb9b637193441e1e2/tok/tok.go#L48
[tok]: https://github.com/dgraph-io/dgraph/blob/ce82aaafba3d9e57cf5ea1aeb9b637193441e1e2/tok/tok.go#L37-L53

### Key Lookup

Every key can be inspected further with the `--lookup` flag for the specific key.

```sh
$ dgraph debug --postings ./p --lookup 00000b6465736372697074696f6e020866617374
```

```text
Opening DB: ./p
Min commit: 1. Max commit: 5, w.r.t 18446744073709551615
 Key: 00000b6465736372697074696f6e0208676f Length: 2
 Uid: 1 Op: 1
 Uid: 2 Op: 1
```

For data keys, a lookup shows its type and value. Below, we see that the key for `attr: url uid: 1` is a string value.

```sh
$ dgraph debug --postings ./p --lookup 00000375726c000000000000000001
```

```text
Opening DB: ./p
Min commit: 1. Max commit: 5, w.r.t 18446744073709551615
 Key: 0000046e616d65000000000000000001 Length: 1
 Uid: 18446744073709551615 Op: 1  Type: STRING.  String Value: "https://github.com/dgraph-io/dgraph"
```

For index keys, a lookup shows the UIDs that are part of this index. Below, we see that the `fast` index for the `<description>` predicate has UIDs 0x1 and 0x2.

```sh
$ dgraph debug --postings ./p --lookup 00000b6465736372697074696f6e020866617374
```

```text
Opening DB: ./p
Min commit: 1. Max commit: 5, w.r.t 18446744073709551615
 Key: 00000b6465736372697074696f6e0208676f Length: 2
 Uid: 1 Op: 1
 Uid: 2 Op: 1
```

### Key History

You can also look up the history of values for a key using the `--history` option.

```sh
$ dgraph debug --postings ./p --lookup 00000b6465736372697074696f6e020866617374 --history
```
```text
Opening DB: ./p
Min commit: 1. Max commit: 5, w.r.t 18446744073709551615
==> key: 00000b6465736372697074696f6e020866617374. PK: &{byteType:2 Attr:description Uid:0 Termfast Count:0 bytePrefix:0}
ts: 5 {item}{delta}
 Uid: 2 Op: 1

ts: 3 {item}{delta}
 Uid: 1 Op: 1
```

Above, we see that UID 0x1 was committed to this index at ts 3, and UID 0x2 was committed to this index at ts 5.

The debug output also shows UserMeta information:

- `{complete}`: Complete posting list
- `{uid}`: UID posting list
- `{delta}`: Delta posting list
- `{empty}`: Empty posting list
- `{item}`: Item posting list
- `{deleted}`: Delete marker

## Using the Increment Tool

The `dgraph increment` tool increments a counter value transactionally. The
increment tool can be used as a health check that an Alpha is able to service
transactions for both queries and mutations.

### Example Usage

Increment the default predicate (`counter.val`) once. If the predicate doesn't yet
exist, then it will be created starting at counter 0.

```sh
$ dgraph increment
```

Increment the counter predicate against the Alpha running at address `--alpha` (default: `localhost:9080`):

```sh
$ dgraph increment --alpha=192.168.1.10:9080
```

Increment the counter predicate specified by `--pred` (default: `counter.val`):

```sh
$ dgraph increment --pred=counter.val.healthcheck
```

Run a read-only query for the counter predicate and does not run a mutation to increment it:

```sh
$ dgraph increment --ro
```

Run a best-effort query for the counter predicate and does not run a mutation to increment it:

```sh
$ dgraph increment --be
```

Run the increment tool 1000 times every 1 second:

```sh
$ dgraph increment --num=1000 --wait=1s
```

### Increment Tool Output

```sh
# Run increment a few times
$ dgraph increment
0410 10:31:16.379 Counter VAL: 1   [ Ts: 1 ]
$ dgraph increment
0410 10:34:53.017 Counter VAL: 2   [ Ts: 3 ]
$ dgraph increment
0410 10:34:53.648 Counter VAL: 3   [ Ts: 5 ]

# Run read-only queries to read the counter a few times
$ dgraph increment --ro
0410 10:34:57.35  Counter VAL: 3   [ Ts: 7 ]
$ dgraph increment --ro
0410 10:34:57.886 Counter VAL: 3   [ Ts: 7 ]
$ dgraph increment --ro
0410 10:34:58.129 Counter VAL: 3   [ Ts: 7 ]

# Run best-effort query to read the counter a few times
$ dgraph increment --be
0410 10:34:59.867 Counter VAL: 3   [ Ts: 7 ]
$ dgraph increment --be
0410 10:35:01.322 Counter VAL: 3   [ Ts: 7 ]
$ dgraph increment --be
0410 10:35:02.674 Counter VAL: 3   [ Ts: 7 ]

# Run a read-only query to read the counter 5 times
$ dgraph increment --ro --num=5
0410 10:35:18.812 Counter VAL: 3   [ Ts: 7 ]
0410 10:35:18.813 Counter VAL: 3   [ Ts: 7 ]
0410 10:35:18.815 Counter VAL: 3   [ Ts: 7 ]
0410 10:35:18.817 Counter VAL: 3   [ Ts: 7 ]
0410 10:35:18.818 Counter VAL: 3   [ Ts: 7 ]

# Increment the counter 5 times
$ dgraph increment --num=5
0410 10:35:24.028 Counter VAL: 4   [ Ts: 8 ]
0410 10:35:24.061 Counter VAL: 5   [ Ts: 10 ]
0410 10:35:24.104 Counter VAL: 6   [ Ts: 12 ]
0410 10:35:24.145 Counter VAL: 7   [ Ts: 14 ]
0410 10:35:24.178 Counter VAL: 8   [ Ts: 16 ]

# Increment the counter 5 times, once every second.
$ dgraph increment --num=5 --wait=1s
0410 10:35:26.95  Counter VAL: 9   [ Ts: 18 ]
0410 10:35:27.975 Counter VAL: 10   [ Ts: 20 ]
0410 10:35:28.999 Counter VAL: 11   [ Ts: 22 ]
0410 10:35:30.028 Counter VAL: 12   [ Ts: 24 ]
0410 10:35:31.054 Counter VAL: 13   [ Ts: 26 ]

# If the Alpha is too busy or unhealthy, the tool will timeout and retry.
$ dgraph increment
0410 10:36:50.857 While trying to process counter: Query error: rpc error: code = DeadlineExceeded desc = context deadline exceeded. Retrying...
```

## Giving Nodes a Type

It's often useful to give the nodes in a graph *types* (also commonly referred
to as *labels* or *kinds*).

This allows you to do lots of useful things. For example:

- Search for all nodes of a certain type in the root function.

- Filter nodes to only be of a certain kind.

- Enable easier exploration and understanding of a dataset. Graphs are easier
  to grok when there's an explicit type for each node, since there's a clearer
expectation about what predicates it may or may not have.

- Allow users coming from traditional SQL-like RDBMSs will feel more at home;
  traditional tables naturally map to node types.

The best solution for adding node kinds is to associate each type of node with
a particular predicate. E.g. type *foo* is associated with a predicate `foo`,
and type *bar* is associated with a predicate `bar`. The schema doesn't matter
too much. I can be left as the default schema, and the value given to it can
just be `""`.

The [`has`](http://localhost:1313/query-language/#has) function can be used for
both searching at the query root and filtering inside the query.

To search for all *foo* nodes, follow a predicate, then filter for only *bar*
nodes:
```json
{
  q(func: has(foo)) {
    pred @filter(bar) {
      ...
    }
  }
}
```

Another approach is to have a `type` predicate with schema type `string`,
indexed with the `exact` tokenizer. `eq(type, "foo")` and `@filter(eq(type,
"foo"))` can be used to search and filter. **This second approach has some
serious drawbacks** (especially since the introduction of transactions in
v0.9).  It's **recommended instead to use the first approach.**

The first approach has better scalability properties. Because it uses many
predicates rather than just one, it allows better predicate balancing on
multi-node clusters. The second approach will also result in an increased
transaction abortion rate, since every typed node creation would result in
writing to the `type` index.

## Loading CSV Data

[Dgraph mutations]({{< relref "mutations/index.md" >}}) are accepted in RDF
N-Quad and JSON formats. To load CSV-formatted data into Dgraph, first convert
the dataset into one of the accepted formats and then load the resulting dataset
into Dgraph. This section demonstrates converting CSV into JSON. There are
many tools available to convert CSV to JSON. For example, you can use
[`d3-dsv`](https://github.com/d3/d3-dsv)'s `csv2json` tool as shown below:

```csv
Name,URL
Dgraph,https://github.com/dgraph-io/dgraph
Badger,https://github.com/dgraph-io/badger
```

```sh
$ csv2json names.csv --out names.json
$ cat names.json | jq '.'
[
  {
    "Name": "Dgraph",
    "URL": "https://github.com/dgraph-io/dgraph"
  },
  {
    "Name": "Badger",
    "URL": "https://github.com/dgraph-io/badger"
  }
]
```

This JSON can be loaded into Dgraph via the programmatic clients. This follows
the [JSON Mutation Format]({{< relref "mutations#json-mutation-format" >}}).
Note that each JSON object in the list above will be assigned a unique UID since
the `uid` field is omitted.

[The Ratel UI (and HTTP clients) expect JSON data to be stored within the `"set"`
key]({{< relref "mutations/index.md#json-syntax-using-raw-http-or-ratel-ui"
>}}). You can use `jq` to transform the JSON into the correct format:

```sh
$ cat names.json | jq '{ set: . }'
```
```json
{
  "set": [
    {
      "Name": "Dgraph",
      "URL": "https://github.com/dgraph-io/dgraph"
    },
    {
      "Name": "Badger",
      "URL": "https://github.com/dgraph-io/badger"
    }
  ]
}
```

Let's say you have CSV data in a file named connects.csv that's connecting nodes
together. Here, the `connects` field should `uid` type.

```csv
uid,connects
_:a,_:b
_:a,_:c
_:c,_:d
_:d,_:a
```

{{% notice "note" %}}
To reuse existing integer IDs from a CSV file as UIDs in Dgraph, use Dgraph Zero's [assign endpoint]({{< relref "deploy/index.md#more-about-dgraph-zero" >}}) before data loading to allocate a range of UIDs that can be safely assigned.
{{% /notice %}}

To get the correct JSON format, you can convert the CSV into JSON and use `jq`
to transform it in the correct format where the `connects` edge is a node uid:

```sh
$ csv2json connects.csv | jq '[ .[] | { uid: .uid, connects: { uid: .connects } } ]'
```

```json
[
  {
    "uid": "_:a",
    "connects": {
      "uid": "_:b"
    }
  },
  {
    "uid": "_:a",
    "connects": {
      "uid": "_:c"
    }
  },
  {
    "uid": "_:c",
    "connects": {
      "uid": "_:d"
    }
  },
  {
    "uid": "_:d",
    "connects": {
      "uid": "_:a"
    }
  }
]
```

You can modify the `jq` transformation to output the mutation format accepted by
Ratel UI and HTTP clients:

```sh
$ csv2json connects.csv | jq '{ set: [ .[] | {uid: .uid, connects: { uid: .connects } } ] }'
```
```json
{
  "set": [
    {
      "uid": "_:a",
      "connects": {
        "uid": "_:b"
      }
    },
    {
      "uid": "_:a",
      "connects": {
        "uid": "_:c"
      }
    },
    {
      "uid": "_:c",
      "connects": {
        "uid": "_:d"
      }
    },
    {
      "uid": "_:d",
      "connects": {
        "uid": "_:a"
      }
    }
  ]
}
```

## A Simple Login System

{{% notice "note" %}}
This example is based on part of the [transactions in
v0.9](https://blog.dgraph.io/post/v0.9/) blogpost. Error checking has been
omitted for brevity.
{{% /notice %}}

Schema is assumed to be:
```
// @upsert directive is important to detect conflicts.
email: string @index(exact) @upsert . # @index(hash) would also work
pass: password .
```

```
// Create a new transaction. The deferred call to Discard
// ensures that server-side resources are cleaned up.
txn := client.NewTxn()
defer txn.Discard(ctx)

// Create and execute a query to looks up an email and checks if the password
// matches.
q := fmt.Sprintf(`
    {
        login_attempt(func: eq(email, %q)) {
            checkpwd(pass, %q)
        }
    }
`, email, pass)
resp, err := txn.Query(ctx, q)

// Unmarshal the response into a struct. It will be empty if the email couldn't
// be found. Otherwise it will contain a bool to indicate if the password matched.
var login struct {
    Account []struct {
        Pass []struct {
            CheckPwd bool `json:"checkpwd"`
        } `json:"pass"`
    } `json:"login_attempt"`
}
err = json.Unmarshal(resp.GetJson(), &login); err != nil {

// Now perform the upsert logic.
if len(login.Account) == 0 {
    fmt.Println("Account doesn't exist! Creating new account.")
    mu := &protos.Mutation{
        SetJson: []byte(fmt.Sprintf(`{ "email": %q, "pass": %q }`, email, pass)),
    }
    _, err = txn.Mutate(ctx, mu)
    // Commit the mutation, making it visible outside of the transaction.
    err = txn.Commit(ctx)
} else if login.Account[0].Pass[0].CheckPwd {
    fmt.Println("Login successful!")
} else {
    fmt.Println("Wrong email or password.")
}
```

## Upserts

Upsert-style operations are operations where:

1. A node is searched for, and then
2. Depending on if it is found or not, either:
    - Updating some of its attributes, or
    - Creating a new node with those attributes.

The upsert has to be an atomic operation such that either a new node is
created, or an existing node is modified. It's not allowed that two concurrent
upserts both create a new node.

There are many examples where upserts are useful. Most examples involve the
creation of a 1 to 1 mapping between two different entities. E.g. associating
email addresses with user accounts.

Upserts are common in both traditional RDBMSs and newer NoSQL databases.
Dgraph is no exception.

### Upsert Procedure

In Dgraph, upsert-style behaviour can be implemented by users on top of
transactions. The steps are as follows:

1. Create a new transaction.

2. Query for the node. This will usually be as simple as `{ q(func: eq(email,
   "bob@example.com")) { uid }}`. If a `uid` result is returned, then that's the
`uid` for the existing node. If no results are returned, then the user account
doesn't exist.

3. In the case where the user account doesn't exist, then a new node has to be
   created. This is done in the usual way by making a mutation (inside the
transaction), e.g.  the RDF `_:newAccount <email> "bob@example.com" .`. The
`uid` assigned can be accessed by looking up the blank node name `newAccount`
in the `Assigned` object returned from the mutation.

4. Now that you have the `uid` of the account (either new or existing), you can
   modify the account (using additional mutations) or perform queries on it in
whichever way you wish.

### Conflicts

Upsert operations are intended to be run concurrently, as per the needs of the
application. As such, it's possible that two concurrently running operations
could try to add the same node at the same time. For example, both try to add a
user with the same email address. If they do, then one of the transactions will
fail with an error indicating that the transaction was aborted.

If this happens, the transaction is rolled back and it's up to the user's
application logic to retry the whole operation. The transaction has to be
retried in its entirety, all the way from creating a new transaction.

The choice of index placed on the predicate is important for performance.
**Hash is almost always the best choice of index for equality checking.**

{{% notice "note" %}}
It's the _index_ that typically causes upsert conflicts to occur. The index is
stored as many key/value pairs, where each key is a combination of the
predicate name and some function of the predicate value (e.g. its hash for the
hash index). If two transactions modify the same key concurrently, then one
will fail.
{{% /notice %}}

## Run Jepsen tests

1. Clone the jepsen repo at [https://github.com/jepsen-io/jepsen](https://github.com/jepsen-io/jepsen).

```sh
git clone git@github.com:jepsen-io/jepsen.git
```

2. Run the following command to setup the instances from the repo.

```sh
cd docker && ./up.sh
```

This should start 5 jepsen nodes in docker containers.

3. Now ssh into `jepsen-control` container and run the tests.

{{% notice "note" %}}
You can use the [transfer](https://github.com/dgraph-io/dgraph/blob/master/contrib/nightly/transfer.sh) script to build the Dgraph binary and upload the tarball to https://transfer.sh, which gives you a url that can then be used in the Jepsen tests (using --package-url flag).
{{% /notice %}}



```sh
docker exec -it jepsen-control bash
```

```sh
root@control:/jepsen# cd dgraph
root@control:/jepsen/dgraph# lein run test -w upsert

# Specify a --package-url

root@control:/jepsen/dgraph# lein run test --force-download --package-url https://github.com/dgraph-io/dgraph/releases/download/nightly/dgraph-linux-amd64.tar.gz -w upsert
```
