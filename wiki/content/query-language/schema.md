+++
date = "2017-03-20T22:25:17+11:00"
title = "Schema"
weight = 20
[menu.main]
    parent = "query-language"
+++

For each predicate, the schema specifies the target's type.  If a predicate `p` has type `T`, then for all subject-predicate-object triples `s p o` the object `o` is of schema type `T`.

* On mutations, scalar types are checked and an error thrown if the value cannot be converted to the schema type.

* On query, value results are returned according to the schema type of the predicate.

If a schema type isn't specified before a mutation adds triples for a predicate, then the type is inferred from the first mutation.  This type is either:

* type `uid`, if the first mutation for the predicate has nodes for the subject and object, or

* derived from the [RDF type]({{< relref "#rdf-types" >}}), if the object is a literal and an RDF type is present in the first mutation, or

* `default` type, otherwise.


## Schema Types

Dgraph supports scalar types and the UID type.

### Scalar Types

For all triples with a predicate of scalar types the object is a literal.

| Dgraph Type | Go type |
| ------------|:--------|
|  `default`  | string  |
|  `int`      | int64   |
|  `float`    | float   |
|  `string`   | string  |
|  `bool`     | bool    |
|  `dateTime` | time.Time (RFC3339 format [Optional timezone] eg: 2006-01-02T15:04:05.999999999+10:00 or 2006-01-02T15:04:05.999999999)    |
|  `geo`      | [go-geom](https://github.com/twpayne/go-geom)    |
|  `password` | string (encrypted) |


{{% notice "note" %}}Dgraph supports date and time formats for `dateTime` scalar type only if they
are RFC 3339 compatible which is different from ISO 8601(as defined in the RDF spec). You should
convert your values to RFC 3339 format before sending them to Dgraph.{{% /notice  %}}

### UID Type

The `uid` type denotes a node-node edge; internally each node is represented as a `uint64` id.

| Dgraph Type | Go type |
| ------------|:--------|
|  `uid`      | uint64  |


## Adding or Modifying Schema

Schema mutations add or modify schema.

Multiple scalar values can also be added for a `S P` by specifying the schema to be of
list type. Occupations in the example below can store a list of strings for each `S P`.

An index is specified with `@index`, with arguments to specify the tokenizer. When specifying an
index for a predicate it is mandatory to specify the type of the index. For example:

```
name: string @index(exact, fulltext) @count .
multiname: string @lang .
age: int @index(int) .
friend: [uid] @count .
dob: dateTime .
location: geo @index(geo) .
occupations: [string] @index(term) .
```

If no data has been stored for the predicates, a schema mutation sets up an empty schema ready to receive triples.

If data is already stored before the mutation, existing values are not checked to conform to the new schema.  On query, Dgraph tries to convert existing values to the new schema types, ignoring any that fail conversion.

If data exists and new indices are specified in a schema mutation, any index not in the updated list is dropped and a new index is created for every new tokenizer specified.

Reverse edges are also computed if specified by a schema mutation.

{{% notice "note" %}}You can't define predicate names starting with `dgraph.`, it is reserved as the
namespace for Dgraph's internal types/predicates. For example, defining `dgraph.name` as a
predicate is invalid.{{% /notice  %}}


## Indexes in Background

Indexes may take long time to compute depending upon the size of the data.
Starting Dgraph version `20.03.0`, indexes can be computed in the background,
and thus indexing may still be running after an Alter operation returns.
This requires that you wait for indexing to complete before running queries
that require newly created indices. Such queries will fail with an error
notifying that a given predicate is not indexed or doesn't have reverse edges.

An alter operation will also fail if one is already in progress with an error
`schema is already being modified. Please retry`. Though, mutations can
be successfully executed while indexing is going on.

For example, let's say we execute an Alter operation with the following schema:

```
name: string @index(fulltext, term) .
age: int @index(int) @upsert .
friend: [uid] @count @reverse .
```

Once the Alter operation returns, Dgraph will report the following schema
and start background tasks to compute all the new indexes:

```
name: string .
age: int @upsert .
friend: [uid] .
```

When indexes are done computing, Dgraph will start reporting the indexes in the
schema. In a multi-node cluster, it is possible that the alphas will finish
computing indexes at different times. Alphas may return different schema in such
a case until all the indexes are done computing on all the Alphas.

Background indexing task may fail if an unexpected error occurs while computing
the indexes. You should retry the Alter operation in order to update the schema,
or sync the schema across all the alphas.

We also plan to add a simpler API soon to check the status of background indexing.
See this [PR](https://github.com/dgraph-io/dgraph/pull/4961) for more details.

### HTTP API

You can specify the flag `runInBackground` to `true` to run
index computation in the background.

```sh
curl localhost:8080/alter?runInBackground=true -XPOST -d $'
    name: string @index(fulltext, term) .
    age: int @index(int) @upsert .
    friend: [uid] @count @reverse .
' | python -m json.tool | less
```

### Grpc API

You can set `RunInBackground` field to `true` of the `api.Operation`
struct before passing it to the `Alter` function.

```go
op := &api.Operation{}
op.Schema = `
  name: string @index(fulltext, term) .
  age: int @index(int) @upsert .
  friend: [uid] @count @reverse .
`
op.RunInBackground = true
err = dg.Alter(context.Background(), op)
```


## Predicate name rules

Any alphanumeric combination of a predicate name is permitted.
Dgraph also supports [Internationalized Resource Identifiers](https://en.wikipedia.org/wiki/Internationalized_Resource_Identifier) (IRIs).
You can read more in [Predicates i18n](#predicates-i18n).

### Allowed special characters

Single special characters are not accepted, which includes the special characters from IRIs.
They have to be prefixed/suffixed with alphanumeric characters.

```
][&*()_-+=!#$%
```

*Note: You are not restricted to use @ suffix, but the suffix character gets ignored.*

### Forbidden special characters

The special characters below are not accepted.

```
^}|{`\~
```


## Predicates i18n

If your predicate is a URI or has language-specific characters, then enclose
it with angle brackets `<>` when executing the schema mutation.

{{% notice "note" %}}Dgraph supports [Internationalized Resource Identifiers](https://en.wikipedia.org/wiki/Internationalized_Resource_Identifier) (IRIs) for predicate names and values.{{% /notice  %}}

Schema syntax:
```
<职业>: string @index(exact) .
<年龄>: int @index(int) .
<地点>: geo @index(geo) .
<公司>: string .
```

This syntax allows for internationalized predicate names, but full-text indexing still defaults to English.
To use the right tokenizer for your language, you need to use the `@lang` directive and enter values using your
language tag.

Schema:
```
<公司>: string @index(fulltext) @lang .
```
Mutation:
```
{
  set {
    _:a <公司> "Dgraph Labs Inc"@en .
    _:b <公司> "夏新科技有限责任公司"@zh .
    _:a <dgraph.type> "Company" .
  }
}
```
Query:
```
{
  q(func: alloftext(<公司>@zh, "夏新科技有限责任公司")) {
    uid
    <公司>@.
  }
}
```


## Upsert directive

To use [upsert operations]({{< relref "howto/upserts.md">}}) on a
predicate, specify the `@upsert` directive in the schema. When committing
transactions involving predicates with the `@upsert` directive, Dgraph checks
index keys for conflicts, helping to enforce uniqueness constraints when running
concurrent upserts.

This is how you specify the upsert directive for a predicate.
```
email: string @index(exact) @upsert .
```

## Noconflict directive

The NoConflict directive prevents conflict detection at the predicate level. This is an experimental feature and not a
recommended directive but exists to help avoid conflicts for predicates that don't have high
correctness requirements. This can cause data loss, especially when used for predicates with count
index.

This is how you specify the `@noconflict` directive for a predicate.
```
email: string @index(exact) @noconflict .
```

## RDF Types

Dgraph supports a number of [RDF types in mutations]({{< relref "mutations/language-rdf-types.md" >}}).

As well as implying a schema type for a [first mutation]({{< relref "query-language/schema.md" >}}), an RDF type can override a schema type for storage.

If a predicate has a schema type and a mutation has an RDF type with a different underlying Dgraph type, the convertibility to schema type is checked, and an error is thrown if they are incompatible, but the value is stored in the RDF type's corresponding Dgraph type.  Query results are always returned in schema type.

For example, if no schema is set for the `age` predicate.  Given the mutation
```
{
 set {
  _:a <age> "15"^^<xs:int> .
  _:b <age> "13" .
  _:c <age> "14"^^<xs:string> .
  _:d <age> "14.5"^^<xs:string> .
  _:e <age> "14.5" .
 }
}
```
Dgraph:

* sets the schema type to `int`, as implied by the first triple,
* converts `"13"` to `int` on storage,
* checks `"14"` can be converted to `int`, but stores as `string`,
* throws an error for the remaining two triples, because `"14.5"` can't be converted to `int`.

## Extended Types

The following types are also accepted.

### Password type

A password for an entity is set with setting the schema for the attribute to be of type `password`.  Passwords cannot be queried directly, only checked for a match using the `checkpwd` function.
The passwords are encrypted using [bcrypt](https://en.wikipedia.org/wiki/Bcrypt).

For example: to set a password, first set schema, then the password:
```
pass: password .
```

```
{
  set {
    <0x123> <name> "Password Example" .
    <0x123> <pass> "ThePassword" .
  }
}
```

to check a password:
```
{
  check(func: uid(0x123)) {
    name
    checkpwd(pass, "ThePassword")
  }
}
```

output:
```
{
  "data": {
    "check": [
      {
        "name": "Password Example",
        "checkpwd(pass)": true
      }
    ]
  }
}
```

You can also use alias with password type.

```
{
  check(func: uid(0x123)) {
    name
    secret: checkpwd(pass, "ThePassword")
  }
}
```

output:
```
{
  "data": {
    "check": [
      {
        "name": "Password Example",
        "secret": true
      }
    ]
  }
}
```

## Indexing

{{% notice "note" %}}Filtering on a predicate by applying a [function]({{< relref "query-language/functions.md" >}}) requires an index.{{% /notice %}}

When filtering by applying a function, Dgraph uses the index to make the search through a potentially large dataset efficient.

All scalar types can be indexed.

Types `int`, `float`, `bool` and `geo` have only a default index each: with tokenizers named `int`, `float`, `bool` and `geo`.

Types `string` and `dateTime` have a number of indices.

### String Indices
The indices available for strings are as follows.

| Dgraph function            | Required index / tokenizer             | Notes |
| :-----------------------   | :------------                          | :---  |
| `eq`                       | `hash`, `exact`, `term`, or `fulltext` | The most performant index for `eq` is `hash`. Only use `term` or `fulltext` if you also require term or full-text search. If you're already using `term`, there is no need to use `hash` or `exact` as well. |
| `le`, `ge`, `lt`, `gt`     | `exact`                                | Allows faster sorting.                                   |
| `allofterms`, `anyofterms` | `term`                                 | Allows searching by a term in a sentence.                |
| `alloftext`, `anyoftext`   | `fulltext`                             | Matching with language specific stemming and stopwords.  |
| `regexp`                   | `trigram`                              | Regular expression matching. Can also be used for equality checking. |

{{% notice "warning" %}}
Incorrect index choice can impose performance penalties and an increased
transaction conflict rate. Use only the minimum number of and simplest indexes
that your application needs.
{{% /notice %}}


### DateTime Indices

The indices available for `dateTime` are as follows.

| Index name / Tokenizer   | Part of date indexed                                      |
| :----------- | :------------------------------------------------------------------ |
| `year`      | index on year (default)                                        |
| `month`       | index on year and month                                         |
| `day`       | index on year, month and day                                      |
| `hour`       | index on year, month, day and hour                               |

The choices of `dateTime` index allow selecting the precision of the index.  Applications, such as the movies examples in these docs, that require searching over dates but have relatively few nodes per year may prefer the `year` tokenizer; applications that are dependent on fine grained date searches, such as real-time sensor readings, may prefer the `hour` index.


All the `dateTime` indices are sortable.


### Sortable Indices

Not all the indices establish a total order among the values that they index. Sortable indices allow inequality functions and sorting.

* Indexes `int` and `float` are sortable.
* `string` index `exact` is sortable.
* All `dateTime` indices are sortable.

For example, given an edge `name` of `string` type, to sort by `name` or perform inequality filtering on names, the `exact` index must have been specified.  In which case a schema query would return at least the following tokenizers.

```
{
  "predicate": "name",
  "type": "string",
  "index": true,
  "tokenizer": [
    "exact"
  ]
}
```

### Count index

For predicates with the `@count` Dgraph indexes the number of edges out of each node.  This enables fast queries of the form:
```
{
  q(func: gt(count(pred), threshold)) {
    ...
  }
}
```

## List Type

Predicate with scalar types can also store a list of values if specified in the schema. The scalar
type needs to be enclosed within `[]` to indicate that its a list type.

```
occupations: [string] .
score: [int] .
```

* A set operation adds to the list of values. The order of the stored values is non-deterministic.
* A delete operation deletes the value from the list.
* Querying for these predicates would return the list in an array.
* Indexes can be applied on predicates which have a list type and you can use [Functions]({{<relref "query-language/functions.md">}}) on them.
* Sorting is not allowed using these predicates.
* These lists are like an unordered set. For example: `["e1", "e1", "e2"]` may get stored as `["e2", "e1"]`, i.e., duplicate values will not be stored and order may not be preserved.

## Filtering on list

Dgraph supports filtering based on the list.
Filtering works similarly to how it works on edges and has the same available functions.

For example, `@filter(eq(occupations, "Teacher"))` at the root of the query or the
parent edge will display all the occupations from a list of each node in an array but
will only include nodes which have `Teacher` as one of the occupations. However, filtering
on value edge is not supported.

## Reverse Edges

A graph edge is unidirectional. For node-node edges, sometimes modeling requires reverse edges.  If only some subject-predicate-object triples have a reverse, these must be manually added.  But if a predicate always has a reverse, Dgraph computes the reverse edges if `@reverse` is specified in the schema.

The reverse edge of `anEdge` is `~anEdge`.

For existing data, Dgraph computes all reverse edges.  For data added after the schema mutation, Dgraph computes and stores the reverse edge for each added triple.

## Querying Schema

A schema query queries for the whole schema:

```
schema {}
```

{{% notice "note" %}} Unlike regular queries, the schema query is not surrounded
by curly braces. Also, schema queries and regular queries cannot be combined.
{{% /notice %}}

You can query for particular schema fields in the query body.

```
schema {
  type
  index
  reverse
  tokenizer
  list
  count
  upsert
  lang
}
```

You can also query for particular predicates:

```
schema(pred: [name, friend]) {
  type
  index
  reverse
  tokenizer
  list
  count
  upsert
  lang
}
```

Types can also be queried. Below are some example queries.

```
schema(type: Movie) {}
schema(type: [Person, Animal]) {}
```

Note that type queries do not contain anything between the curly braces. The
output will be the entire definition of the requested types.
