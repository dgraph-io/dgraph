+++
date = "2017-03-20T19:35:35+11:00"
title = "How To Guides"
+++

## Retrieving Debug Information

Each Dgraph data node exposes profile over `/debug/pprof` endpoint and metrics over `/debug/vars` endpoint. Each Dgraph data node has it's own profiling and metrics information. Below is a list of debugging information exposed by Dgraph and the corresponding commands to retrieve them.

If you are collecting these metrics from outside the dgraph instance you need to pass `--expose_trace=true` flag, otherwise there metrics can be collected by connecting to the instance over localhost.

- Metrics exposed by Dgraph
```
curl http://<IP>:<HTTP_PORT>/debug/vars
```

- Heap Profile
```
go tool pprof http://<IP>:<HTTP_PORT>/debug/pprof/heap
#Fetching profile from ...
#Saved Profile in ...
```
The output of the command would show the location where the profile is stored.

- CPU Profile
```
go tool pprof http://<IP>:<HTTP_PORT>/debug/pprof/profile
```

- Block Profile
Dgraph by default doesn't collect the block profile. Dgraph must be started with `--block=<N>` with N > 1.
```
go tool pprof http://<IP>:<HTTP_PORT>/debug/pprof/block
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
   "bob@example.com") { uid }}`. If a `uid` result is returned, then that's the
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

