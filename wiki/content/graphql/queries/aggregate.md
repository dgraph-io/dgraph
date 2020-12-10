+++
title = "Aggregate Queries"
weight = 3
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
`Sum` and `Avg`). Which aggregate query fields are available depends on the
field's type (`Int`, `Float`, `String` or `Datetime`), as follows:

* **`<field name>Max`** and **`<field name>Min`**: generated for fields typed as
`Int`, `Float`, `String` or `Datetime`.
* **`<field name>Sum`** and **`<field name>Avg`**: generated for fields typed as
`Int` or `Float`.

#### Examples

Example: Fetch the minimum, maximum, and average number of `posts` per
`Author`:

```graphql
   query {
     aggregateAuthor {
       postsMin
       postsMax
       postsAvg
     }
   }
```
Example: Fetch the total number of `posts` by all authors:

```graphql
   query {
     aggregateAuthor {
       postsSum
     }
   }
```



### Aggregate queries for child nodes

Dgraph also defines aggregate `<field name>Aggregate` fields for child nodes
within `query<type name>` queries. This is done for each field that is of type
`List[Type/Interface]` inside `query<type name>` queries, allowing you do get
minimums, maximums, averages and sums for those fields. Which aggregate query
fields are available depends on the field's type (`Int`, `Float`, `String` or
`Datetime`), as follows:

* **`<field name>Max`** and **`<field name>Min`**: generated for fields typed as
`Int`, `Float`, `String` or `Datetime`.
* **`<field name>Sum`** and **`<field name>Avg`**: generated for fields typed as
`Int` or `Float`.

#### Examples

Example: Fetch the minimum, maximum and average `score` of the `posts` for each
author, along with each author's `name`.

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
