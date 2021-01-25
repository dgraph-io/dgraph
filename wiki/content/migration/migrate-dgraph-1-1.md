+++
date = "2017-03-20T22:25:17+11:00"
title = "Migrate from Dgraph v1.0"
weight = 11
[menu.main]
    parent = "migration"
+++

This document summarizes major changes in Dgraph database that you should be
aware of as you migrate your data and client applications from Dgraph v1.0 to
newer Dgraph versions.

## Schema types: scalar `uid` and list `[uid]`

The semantics of predicates of type `uid` has changed since Dgraph v1.0. 
In Dgraph v1.0 all `uid` predicates implied a one-to-many relationship, but now
you can express a one-to-one relationship or a one-to-many relationship. The
following syntax example demonstrates both types of relationships:

```text
friend: [uid] .
best_friend: uid .
```

In the above example, the predicate `friend` allows a one-to-many relationship
(i.e a person can have more than one friend) and the predicate `best_friend` 
allows a one-to-one relationship.

The new `uid` syntax is consistent with that used for other types. For example,
`string` indicates a  single-value string and `[string]` represents many strings.

To migrate existing schemas from Dgraph v1.0 to newer Dgraph versions, update
the schema file from an export so all predicates of type `uid` are changed to
`[uid]`. Then, use the updated schema when loading data into Dgraph v1.1. 

For example, you could start with the following schema in Dgraph v1.0:

```text
name: string .
friend: uid .
```

When exported, you get the following schema, suitable for migration into newer
Dgraph versions:

```text
name: string .
friend: [uid] .
```
## Type system

The new [type system]({{< relref "query-language/type-system.md" >}}) introduced
starting in Dgraph 1.1 should not affect migrating data from a previous version.
However, two features of the query language no longer work as they did
in Dgraph v1.0: `expand()` and `_predicate_`.

### Changes to the `expand()` function

Expand queries will not work until the type system has been properly set up. For
example, the following query will return an empty result in newer Dgraph
versions if the node `0xff` has no type information.

```graphql
{
  me(func: uid(0xff)) {
    expand(_all_)
  }
}
```

To make this query work in newer Dgraph versions, you need to add a type
definition to the schema, and then associate a node with that type using a
mutation.

You can add a type definition using the `/alter` endpoint. Let's assume that
the node shown in the previous example represents a person, with the `Person`
type defined as follows:

```graphql
type Person {
  name
  age
}
```

Next, associate a node with the `Person` type by adding the following RDF 
triple to Dgraph (using a mutation):

```text
<0xff> <dgraph.type> "Person" .
```

With these two steps complete, the results of the query in both Dgraph v1.0 and
newer Dgraph versions should be the same.

### `_predicate_`

In Dgraph v1.0 an "internal predicate" (called `_predicate_`) was used to
associate each node with its predicates. This "internal predicate" has been
removed from newer Dgraph versions; instead, you use the type system to get the
predicates that belong to nodes of a given type.

Because `_predicate_` isn't supported in newer Dgraph versions, you can't
reference it explicitly in queries as you could in Dgraph v1.0. 

For example, the following query returns the predicates of the node `0xff` in
Dgraph v1.0:

```graphql
{
  me(func: uid(0xff)) {
     _predicate_ # NOT available in Dgraph v1.1 and newer versions
  }
}
```

In newer Dgraph versions, there isn't an equivalent to this query. Instead, you
can use the type system to fetch the predicates for a given node.

First, you could query for the types associated with the node, as in the
following example:

```graphql
{
  me(func: uid(0xff)) {
     dgraph.type
  }
}
```

Next, you could fetch the definition of each type in the results using a schema
query, as follows

```graphql
schema(type: Person) {}
```

## Live Loader and Bulk Loader command-line flags

A variety of command-line flags for Dgraph Live Loader and Dgraph Bulk Loader
have changed in newer Dgraph versions.

### File input flags

In Dgraph v1.1, both the Dgraph Live Loader and Dgraph Bulk Loader tools support loading data in either RDF format or JSON format. To simplify the command-line interface for these tools, the `-r`/`--rdfs` flag has been removed in favor of `-f/--files`. The new flag accepts file or directory paths for either data format. By default, the tools will infer the file type based on the file suffix, e.g., `.rdf` and `.rdf.gz` or `.json` and `.json.gz` for RDF data or JSON data, respectively. To ignore the filenames and set the format explicitly, the `--format` flag can be set to `rdf` or `json`.

File input example for Dgraph v1.0:

```sh
dgraph live -r data.rdf.gz
```

File input example for newer Dgraph versions:

```sh
dgraph live -f data.rdf.gz
```

### Dgraph Alpha address flag

For Dgraph Live Loader, the Dgraph Alpha address flag (default: `127.0.0.1:9080`)
has changed from `-d`/`--dgraph` to `-a`/`--alpha`.

Dgraph Alpha address example for Dgraph v1.0:

```sh
dgraph live -d 127.0.0.1:9080
```

Dgraph Alpha address example for newer Dgraph versions:

```sh
dgraph live -a 127.0.0.1:9080
```

## HTTP API

For HTTP API users (e.g., Curl, Postman), the custom Dgraph headers have been
removed in favor of standard HTTP headers and query parameters.

### Queries

There are two accepted `Content-Type` headers for queries over HTTP: `application/dql` or `application/json`.

A `Content-Type` must be set to run a query.

`curl` query example for Dgraph v1.0:

```sh
curl localhost:8080/query -d '{
  q(func: eq(name, "Dgraph")) {
    name
  }
}'
```

`curl` query example for newer Dgraph versions:

```sh
curl -H 'Content-Type: application/graphql+-' localhost:8080/query -d '{
  q(func: eq(name, "Dgraph")) {
    name
  }
}'
```

For queries using [GraphQL Variables]({{< relref "query-language/graphql-variables.md" >}}),
the query must be sent using the `application/json` content type, with the query
and variables sent in a JSON payload:

GraphQL variable example for Dgraph v1.0:

```sh
curl -H 'X-Dgraph-Vars: {"$name": "Alice"}' localhost:8080/query -d 'query qWithVars($name: string) {
  q(func: eq(name, $name)) {
    name
  }
}
```

GraphQL variable example for Dgraph v1.1:

```sh
curl -H 'Content-Type: application/json' localhost:8080/query -d '{
  "query": "query qWithVars($name: string) { q(func: eq(name, $name)) { name } }",
  "variables": {"$name": "Alice"}
}'
```

### Mutations

There are two accepted Content-Type headers for mutations over HTTP: `Content-Type: application/rdf` or `Content-Type: application/json`.

A `Content-Type` must be set to run a mutation.

These Content-Type headers supersede the Dgraph v1.0.x custom header `X-Dgraph-MutationType` to set the mutation type as RDF or JSON.

To commit the mutation immediately, use the query parameter `commitNow=true`. This replaces the custom header `X-Dgraph-CommitNow: true` from Dgraph v1.0.x.

RDF mutation syntax example for Dgraph v1.0:

```sh
curl -H 'X-Dgraph-CommitNow: true' localhost:8080/mutate -d '{
  set {
    _:n <name> "Alice" .
    _:n <dgraph.type> "Person" .
  }
}'
```

RDF mutation syntax example for newer Dgraph versions:

```sh
curl -H 'Content-Type: application/rdf' localhost:8080/mutate?commitNow=true -d '{
  set {
    _:n <name> "Alice" .
    _:n <dgraph.type> "Person" .
  }
}'
```

For JSON mutations, set the `Content-Type` header to `application/json`.

JSON mutation syntax example for Dgraph v1.0:

```sh
curl -H 'X-Dgraph-MutationType: json' -H "X-Dgraph-CommitNow: true" localhost:8080/mutate -d '{
  "set": [
    {
      "name": "Alice"
    }
  ]
}'
```

JSON mutation syntax example for newer Dgraph versions:

```sh
curl -H 'Content-Type: application/json' localhost:8080/mutate?commitNow=true -d '{
  "set": [
    {
      "name": "Alice"
    }
  ]
}'
```
