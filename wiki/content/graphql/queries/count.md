+++
title = "Count Queries"
weight = 3
[menu.main]
    parent = "graphql-queries"
    name = "Count Queries"
+++

Dgraph automatically generates count queries for a given GraphQL schema, enabling you to `count` on predicates, edges, and to count nodes satisfying certain criteria specified using a filter. Count Queries are also compatible with the `@auth` directive and honor the same authorization rules as `query`.

### Count at root

For every `type` defined in GraphQL, Dgraph generates an aggregate query `aggregate<type name>`. This query includes a `count` field.

#### Examples

Example - Fetch the number of `posts`.

```graphql
   query {
     aggregatePost {
       count
     }
   }
```

Example - Fetch the number of `posts` whose titles contain `GraphQL`.

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


### Count for a child

Besides the `aggregate<type name>` query, Dgraph defines `<predicate_name>Aggregate` fields inside `query<type name>` queries, allowing you to do a `count` of predicate edges.

#### Examples

Example - Fetch the number of `posts` for all authors along with their `name`.

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

Example - Fetch the number of `posts` with a `score` greater than `10` for all authors along with their `name`
   
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
