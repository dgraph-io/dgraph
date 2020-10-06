+++
date = "2017-03-20T22:25:17+11:00"
title = "Migrate to Dgraph v1.1"
weight = 11
[menu.main]
    parent = "howto"
+++

## Schema types: scalar `uid` and list `[uid]`

The semantics of predicates of type `uid` has changed in Dgraph 1.1. Whereas before all `uid` predicates implied a one-to-many relationship, now a one-to-one relationship or a one-to-many relationship can be expressed.

```
friend: [uid] .
best_friend: uid .
```

In the above, the predicate `friend` allows a one-to-many relationship (i.e a person can have more than one friend) and the predicate best_friend can be at most a one-to-one relationship.

This syntactic meaning is consistent with the other types, e.g., `string` indicating a single-value string and `[string]` representing many strings. This change makes the `uid` type work similarly to other types.

To migrate existing schemas from Dgraph v1.0 to Dgraph v1.1, update the schema file from an export so all predicates of type `uid` are changed to `[uid]`. Then use the updated schema when loading data into Dgraph v1.1. For example, for the following schema:

```text
name: string .
friend: uid .
```

becomes

```text
name: string .
friend: [uid] .
```
## Type system

The new [type system]({{< relref "query-language/type-system.md" >}}) introduced in Dgraph 1.1 should not affect migrating data from a previous version. However, a couple of features in the query language will not work as they did before: `expand()` and `_predicate_`.

The reason is that the internal predicate that associated each node with its predicates (called `_predicate_`) has been removed. Instead, to get the predicates that belong to a node, the type system is used.

### `expand()`

Expand queries will not work until the type system has been properly set up. For example, the following query will return an empty result in Dgraph 1.1 if the node 0xff has no type information.

```text
{
  me(func: uid(0xff)) {
    expand(_all_)
  }
}
```

To make it work again, add a type definition via the alter endpoint. Let’s assume the node in the previous example represents a person. Then, the basic Person type could be defined as follows:

```text
type Person {
  name
  age
}
```

After that, the node is associated with the type by adding the following RDF triple to Dgraph (using a mutation):

```text
<0xff> <dgraph.type> "Person" .
```

After that, the results of the query in both Dgraph v1.0 and Dgraph v1.1 should be the same.

### `_predicate_`

The other consequence of removing `_predicate_` is that it cannot be referenced explicitly in queries. In Dgraph 1.0, the following query returns the predicates of the node 0xff.

```ql
{
  me(func: uid(0xff)) {
     _predicate_ # NOT available in Dgraph v1.1
  }
}
```

**There’s no exact equivalent of this behavior in Dgraph 1.1**, but the information can be queried by first querying for the types associated with that node with the query

```text
{
  me(func: uid(0xff)) {
     dgraph.type
  }
}
```

And then retrieving the definition of each type in the results using a schema query.

```text
schema(type: Person) {}
```

## Live Loader and Bulk Loader command-line flags

### File input flags
In Dgraph v1.1, both the Dgraph Live Loader and Dgraph Bulk Loader tools support loading data in either RDF format or JSON format. To simplify the command-line interface for these tools, the `-r`/`--rdfs` flag has been removed in favor of `-f/--files`. The new flag accepts file or directory paths for either data format. By default, the tools will infer the file type based on the file suffix, e.g., `.rdf` and `.rdf.gz` or `.json` and `.json.gz` for RDF data or JSON data, respectively. To ignore the filenames and set the format explicitly, the `--format` flag can be set to `rdf` or `json`.

Before (in Dgraph v1.0):

```sh
dgraph live -r data.rdf.gz
```

Now (in Dgraph v1.1):

```sh
dgraph live -f data.rdf.gz
```

### Dgraph Alpha address flag
For Dgraph Live Loader, the flag to specify the Dgraph Alpha address  (default: `127.0.0.1:9080`) has changed from `-d`/`--dgraph` to `-a`/`--alpha`.

Before (in Dgraph v1.0):

```sh
dgraph live -d 127.0.0.1:9080
```

Now (in Dgraph v1.1):

```sh
dgraph live -a 127.0.0.1:9080
```
## HTTP API

For HTTP API users (e.g., Curl, Postman), the custom Dgraph headers have been removed in favor of standard HTTP headers and query parameters.

### Queries

There are two accepted `Content-Type` headers for queries over HTTP: `application/graphql+-` or `application/json`.

A `Content-Type` must be set to run a query.

Before (in Dgraph v1.0):

```sh
curl localhost:8080/query -d '{
  q(func: eq(name, "Dgraph")) {
    name
  }
}'
```

Now (in Dgraph v1.1):

```sh
curl -H 'Content-Type: application/graphql+-' localhost:8080/query -d '{
  q(func: eq(name, "Dgraph")) {
    name
  }
}'
```

For queries using [GraphQL Variables]({{< relref "query-language/graphql-variables.md" >}}), the query must be sent via the `application/json` content type, with the query and variables sent in a JSON payload:

Before (in Dgraph v1.0):

```sh
curl -H 'X-Dgraph-Vars: {"$name": "Alice"}' localhost:8080/query -d 'query qWithVars($name: string) {
  q(func: eq(name, $name)) {
    name
  }
}
```

Now (in Dgraph v1.1):

```sh
curl -H 'Content-Type: application/json' localhost:8080/query -d '{
  "query": "query qWithVars($name: string) { q(func: eq(name, $name)) { name } }",
  "variables": {"$name": "Alice"}
}'
```

### Mutations

There are two accepted Content-Type headers for mutations over HTTP: `Content-Type: application/rdf` or `Content-Type: application/json`.

A `Content-Type` must be set to run a mutation.

These Content-Type headers supercede the Dgraph v1.0.x custom header `X-Dgraph-MutationType` to set the mutation type as RDF or JSON.

To commit the mutation immediately, use the query parameter `commitNow=true`. This replaces the custom header `X-Dgraph-CommitNow: true` from Dgraph v1.0.x.

Before (in Dgraph v1.0)

```sh
curl -H 'X-Dgraph-CommitNow: true' localhost:8080/mutate -d '{
  set {
    _:n <name> "Alice" .
    _:n <dgraph.type> "Person" .
  }
}'
```

Now (in Dgraph v1.1):

```sh
curl -H 'Content-Type: application/rdf' localhost:8080/mutate?commitNow=true -d '{
  set {
    _:n <name> "Alice" .
    _:n <dgraph.type> "Person" .
  }
}'
```

For JSON mutations, set the `Content-Type` header to `application/json`.

Before (in Dgraph v1.0):

```sh
curl -H 'X-Dgraph-MutationType: json' -H "X-Dgraph-CommitNow: true" locahost:8080/mutate -d '{
  "set": [
    {
      "name": "Alice"
    }
  ]
}'
```

Now (in Dgraph v1.1):

```sh
curl -H 'Content-Type: application/json' locahost:8080/mutate?commitNow=true -d '{
  "set": [
    {
      "name": "Alice"
    }
  ]
}'
```
