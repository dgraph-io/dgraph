+++
title = "Overview"
weight = 1
[menu.main]
    parent = "lambda"
    identifier = "lambda-resolvers-overview"
+++

In Dgraph, you can customize the behaviour of your schema by implementing JavaScript lambda resolvers.

Dgraph doesn't execute your custom logic itself. It makes external HTTP requests to a user-defined lambda server. That means, you can deploy your lambda logic into the same Kubernetes cluster as your Dgraph instance. If you want to develop your own server, you can find an implementation of Dgraph Lambda server in our [open-source repository](https://github.com/dgraph-io/dgraph-lambda).

## The `@lambda` directive

There are four places where you can use the `@lambda` directive and thus tell Dgraph where to apply custom logic.

1) You can add lambda fields to your types

```graphql
type MyType {
    ...
    customField: FieldType @lambda
}
```

2) You can add lambda fields to your interfaces

```graphql
interface MyInterface {
        ...
        customField: FieldType @lambda
}
```

3) You can add custom queries to the Query type

```graphql
type Query {
    myCustomQuery(...): QueryResultType @lambda
}
```

4) You can add lambda mutations to the Mutation type

```graphql
type Mutation {
    myCustomMutation(...): MutationResult @lambda
}
```

## Learn more

Find out more about the  `@lambda` [directive](/graphql/lambda/directive), or check out:

* [custom query examples](/graphql/lambda/query)
* [custom mutation examples](/graphql/lambda/mutation), or
* [lambda server setup](/graphql/lambda/server)
