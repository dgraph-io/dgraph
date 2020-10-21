+++
title = "Aggregation"
weight = 2
[menu.main]
    parent = "graphql-queries"
    name = "Aggregation"
+++

In this article you'll learn how to use GraphQL queries to aggregate data from Dgraph.

## Count at root

Dgraph has a query named `aggregate<type name>`. This query includes a `count` field.

For example, taking this GraphQL schema:

```graphql
type Data {
	id: ID!
    name: String!
	intList: [Int]
	stringList: [String]
 	metaData: [Data]
}
```

The aggregate schema looks like:

```graphql
input DataAggregateResult {
  count: Int
  ... 
  ...
}
```

Finally, the aggregate query will be:


```graphql
query {
    aggregateData(filter: DataFilter): DataAggregateResult
}
```

The aggregate GraphQL query could also be rewritten into a DQL query as follows:

```graphql
query {
    aggregateData(func: type(Data)) @filter(/* rewritten filter condition */)) {
        count: count(uid)
    }
}
```

## Count for a child

Besides the `aggregateData` query, to handle a `count` of predicate edges and other aggregation fields, you need to define a `<predicate_name>_aggregate` field inside a `queryData` query.

Taking the following GraphQL schema

```graphql
type Data {
  name: String!
  metaData: [Metadata]
}
```

this is a `count` example:

```graphql
query {
    queryData {
        name
        metaData_aggregate {
            count
        }
    }
}
```

The above query could also be rewritten into DQL as follows:

```graphql
query {
    queryData(func: type(Data))  {
        Data.name
        count(Data.metaData)   /* returns number of items in metaData list*/
    }
}
```

{{% notice "note" %}} 
the `count(Data.metaData)` field will be made a part of the `metaData_aggregate` field when returning a GraphQL response.
This approach is similar to how aggregation queries are handled by other GraphQL providers.
{{% /notice %}}

