+++
title = "Count Queries"
weight = 3
[menu.main]
    parent = "graphql-queries"
    name = "Count Queries"
+++

In this article you'll learn how to use GraphQL queries to aggregate data from a Dgraph database.

## Count at root

For every `type` defined in GraphQL, Dgraph has an aggregate query `aggregate<type name>`. This query includes a `count` field.

For example, taking this GraphQL schema:

```graphql
type Author {
    id: ID!
    name: String!
    reputation: [Int]
    posts: [String]
    metaData: [Metadata]
}
```

The aggregate schema looks like:

```graphql
input AuthorAggregateResult {
  count: Int
}
```

Finally, the aggregate query will be:


```graphql
query {
    aggregateAuthor(filter: AuthorFilter): AuthorAggregateResult
}
```

## Count for a child

Besides the `aggregate<type name>` query, to do a `count` of predicate edges and other aggregation fields, you need to define a `<predicate_name>_aggregate` field inside a `query<type name>` query.

Taking the following GraphQL schema

```graphql
type Author {
    name: String!
    posts: [String]
    metaData: [Metadata]
}
```

this is a `count` example for the `posts` field:

```graphql
query {
    queryAuthor {
        name
        posts_aggregate {
            count
        }
    }
}
```
