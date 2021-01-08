+++
title = "Aggregate Queries"
weight = 3
[menu.main]
    parent = "graphql-queries"
    name = "Aggregate Queries"
+++

Dgraph automatically generates aggregate queries for GraphQL schemas.
Aggregate queries fetch aggregate data, including the following:

* *Count queries* that let you count fields
satisfying certain criteria specified using a filter.
* *Advanced aggregate queries* that let you calculate the maximum, minimum, sum
and average of specified fields.

Aggregate queries are compatible with the `@auth` directive and follow the same
authorization rules as the `query` keyword. You can also use filters with
aggregate queries, as shown in some of the examples provided below.

### Count queries at root

For every `type` defined in a GraphQL schema, Dgraph generates an aggregate query
`aggregate<type name>`. This query includes a `count` field, as well as
[advanced aggregate query fields](#advanced-aggregate-queries-at-root).

#### Examples

Example: Fetch the total number of `posts`.

```graphql
   query {
     aggregatePost {
       count
     }
   }
```

Example: Fetch the number of `posts` whose titles contain `GraphQL`.

```graphql
   query {
     aggregatePost(filter: {
       title: {
         anyofterms: "GraphQL"
         }
       }) {
       count
     }
   }
```


### Count queries for child nodes

Dgraph also defines `<field name>Aggregate` fields for every field which
is of type `List[Type/Interface]` inside `query<type name>` queries, allowing
you to do a `count` on fields, or to use the [advanced aggregate queries](#advanced-aggregate-queries-for-child-nodes).

#### Examples

Example: Fetch the number of `posts` for all authors along with their `name`.

```graphql
   query {
     queryAuthor {
       name
       postsAggregate {
        count
       }
     }
   }
```

Example: Fetch the number of `posts` with a `score` greater than `10` for all
authors, along with their `name`

```graphql
   query {
     queryAuthor {
       name
       postsAggregate(filter: {
         score: {
           gt: 10
         }
       }) {
        count
      }
    }
  }
```

### Advanced aggregate queries at root

For every `type` defined in the GraphQL schema, Dgraph generates an aggregate
query `aggregate<type name>` that includes advanced aggregate query
fields, and also includes a `count` field (see [Count queries at root](#count-queries-at-root)). Dgraph generates one or more advanced aggregate
query fields (`<field-name>Min`, `<field-name>Max`, `<field-name>Sum` and
`<field-name>Avg`) for fields in the schema that are typed as `Int`, `Float`,
`String` and `Datetime`.

{{% notice "note" %}}
Advanced aggregate query fields are generated according to a field's type.
Fields typed as `Int` and `Float` get the following query fields:
`<field name>Max`, `<field name>Min`, `<field name>Sum` and `<field name>Avg`.
Fields typed as `String` and `Datetime` only get the `<field name>Max`,
 `<field name>Min` query fields.
{{% /notice %}}

#### Examples

Example: Fetch the average number of `posts` written by authors:

```graphql
   query {
     aggregateAuthor {
       numPostsAvg
     }
   }
```

Example: Fetch the total number of `posts` by all authors, and the maximum
number of `posts` by any single `Author`:

```graphql
   query {
     aggregateAuthor {
       numPostsSum
       numPostsMax
     }
   }
```

Example: Fetch the average number of `posts` for authors with more than 20
`friends`:

```graphql
   query {
     aggregateAuthor (filter: {
       friends: {
         gt: 20
       }
     }) {
       numPostsAvg
     }
   }
```


### Advanced aggregate queries for child nodes

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

### Aggregate queries on null data

Aggregate queries against empty data return `null`. This is true for both the
`<field name>Aggregate` fields and `aggregate<type name>` queries generated by
Dgraph.

So, in the examples above, the following is true:
* If there are no nodes of type `Author`, the `aggregateAuthor` query will
  return null.
* If an `Author` has not written any posts, the field `postsAggregate` will be
  null for that `Author`.
