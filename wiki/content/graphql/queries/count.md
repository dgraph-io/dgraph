+++
title = "Count Queries"
weight = 3
[menu.main]
    parent = "graphql-queries"
    name = "Count Queries"
+++

In this article you'll learn how to use GraphQL queries to aggregate data from a Dgraph database.

Dgraph automatically generates count queries for a given GraphQL schema, enabling you to `count` on predicates, edges, and other aggregation fields.

### Count at root

For every `type` defined in GraphQL, Dgraph generates an aggregate query `aggregate<type name>`. This query includes a `count` field.

For example, taking this GraphQL schema:

```graphql
type Post {
    id: ID!
    title: String!
    body: String
    score: Int
}

type Author {
    id: ID!
    name: String!
    reputation: [Int]
    posts: [Post]
}
```

#### Examples

1. Root count query without a filter:

   Fetch the number of `posts`.


```graphql
   query {
     aggregatePost {
       count
     }
   }
```

2. With a filter:

   Fetch the number of `posts` whose titles contain `GraphQL`.

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

Besides the `aggregate<type name>` query, Dgraph defines `aggregate_<predicate_name>` fields inside `query<type name>` queries, allowing you to do a `count` of predicate edges and other aggregation fields.

For example, following with the GraphQL schema from the [Count at root](#count-at-root) section,

#### Examples

1. Count query at a child level without a filter:

   Fetch the number of `posts` for all authors along with their `name`.

```graphql
   query {
     queryAuthor {
       name
       aggregate_posts {
        count
       }
     }
   }
```

2. With a filter:

   Fetch the number of `posts` with a `score` greater than `10` for all authors along with their `name`
   
```graphql
   query {
     queryAuthor {
       name
       aggregate_posts(filter: {
         score: {
           gt: 10
         }
       }) {
        count
      }
    }
  }
```
