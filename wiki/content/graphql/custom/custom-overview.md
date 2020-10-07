+++
title = "Overview"
weight = 1
[menu.main]
    parent = "custom"
    identifier = "custom-resolvers-overview"
+++

Dgraph creates a GraphQL API from nothing more than GraphQL types.  That's great, and gets you moving fast from an idea to a running app.  However, at some point, as your app develops, you might want to customize the behaviour of your schema.

In Dgraph, you do that with code (in any language you like) that implements custom resolvers.

Dgraph doesn't execute your custom logic itself.  It makes external HTTP requests.  That means, you can deploy your custom logic into the same Kubernetes cluster as your Dgraph instance, deploy and call, for example, AWS Lambda functions, or even make calls to existing HTTP and GraphQL endpoints.

## The `@custom` directive

There are three places you can use the `@custom` directive and thus tell Dgraph where to apply custom logic.

1) You can add custom queries to the Query type

```graphql
type Query {
    myCustomQuery(...): QueryResultType @custom(...)
}
```

2) You can add custom mutations to the Mutation type

```graphql
type Mutation {
    myCustomMutation(...): MutationResult @custom(...)
}
```

3) You can add custom fields to your types

```graphql
type MyType {
    ...
    customField: FieldType @custom(...)
    ...
}
```

## Learn more

Find out more about the  `@custom` directive [here](/graphql/custom/directive), or check out:

* [custom query examples](/graphql/custom/query)
* [custom mutation examples](/graphql/custom/mutation), or
* [custom field examples](/graphql/custom/field)




---
