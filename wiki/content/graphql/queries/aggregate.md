+++
title = "Aggregate Queries"
weight = 4
[menu.main]
    parent = "graphql-queries"
    name = "Aggregate Queries"
+++

Dgraph automatically generates aggregate queries for GraphQL schemas,
letting you calculate the maximum, minimum, sum and average of specified fields,
as well as nodes that meet criteria specified using a filter.

Aggregate queries are compatible with the `@auth` directive and honor the same
authorization rules as the `query` keyword.

### Aggregate queries at root

For every `type` defined in the GraphQL schema, Dgraph generates an aggregate
query `aggregate<type name>`. This query includes a `count` field (to learn
more, see  [Count Queries](/graphql/queries/count/)). Additional fields defined
for each type get one or more additional aggregate query fields (`Min`, `Max`,
`Sum` and `Avg`).

{{% notice "note" %}}
Aggregate query fields are generated according to a field's type. Fields typed
as `Int` and `Float` get the following query fields:`<field name>Max`,
`<field name>Min`, `<field name>Sum` and `<field name>Avg`. Fields typed as
`String` and `Datetime` only get the `<field name>Max`, `<field name>Min` query
fields.
{{% /notice %}}

#### Examples

Example: Fetch the average number of `posts` per `Author`:

```graphql
   query {
     aggregateAuthor {
       postsAvg
     }
   }
```
Example: Fetch the total number of `posts` by all authors, and the maximum
number of `posts` by any single `Author`:

```graphql
   query {
     aggregateAuthor {
       postsSum
       postsMax
     }
   }
```



### Aggregate queries for child nodes

Dgraph also defines aggregate `<field name>Aggregate` fields for child nodes
within `query<type name>` queries. This is done for each field that is of type
`List[Type/Interface]` inside `query<type name>` queries, letting you fetch
minimums, maximums, averages and sums for those fields.

{{% notice "note" %}}
Aggregate query fields are generated according to a field's type. Fields typed
as `Int` and `Float` get the following query fields:`<field name>Max`,
`<field name>Min`, `<field name>Sum` and `<field name>Avg`. Fields typed as
`String` and `Datetime` only get the `<field name>Max`, `<field name>Min` query
fields.
{{% /notice %}}

#### Examples

Example: Fetch the minimum, maximum and average `score` of the `posts` for each
`Author`, along with each author's `name`.

```graphql
   query {
     queryAuthor {
       name
       postsAggregate {
         scoreMin
         scoreMax
         scoreAvg
       }
     }
   }
```

Example: Fetch the date of the most recent post with a `score` greater than
`10` for all authors, along with the author's `name`.

```graphql
   query {
     queryAuthor {
       name
       postsAggregate(filter: {
         score: {
           gt: 10
         }
       }) {
         datePublishedMax
      }
    }
  }
```

### Aggregate query fields available by type
