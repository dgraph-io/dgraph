+++
date = "2017-03-20T19:35:35+11:00"
title = "How To Guides"
+++

### Retrieving Debug Information

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

### Giving Nodes a Type

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

### A Simple Login System

{{% notice "note" %}}
This example is based on part of the [transactions in
v0.9](https://blog.dgraph.io/post/v0.9/) blogpost. Error checking has been
omitted for brevity.
{{% /notice %}}

Schema is assumed to be:
```
email: string @index(exact) . # @index(hash) would also work
pass: password .
```

```
// Create a new transaction. The deferred call to Discard
// ensures that server-side resources are cleaned up.
txn := client.NewTxn()
defer txn.Discard(ctx)

// Create and execute a query to looks up an email and checks if the password
matches.
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
