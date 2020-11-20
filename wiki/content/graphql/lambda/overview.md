+++
title = "Overview"
weight = 1
[menu.main]
    parent = "lambda"
    identifier = "lambda-resolvers-overview"
+++

Lambda provides a way to write your custom logic in JavaScript, integrate it with your GraphQL schema, and execute it using the GraphQL API in a few easy steps:

- Setup a Dgraph cluster with a working lambda server (not required for [Slash GraphQL](dgraph.io/slash-graphql) users)
- Declare lambda queries, mutations, and fields in your GraphQL schema as needed
- Define lambda resolvers for them in a JavaScript file

This also simplifies the job of UI developers, allowing them to work in JavaScript, without knowing any backend language.

Dgraph doesn't execute your custom logic itself. It makes external HTTP requests to a user-defined lambda server. That means, you can deploy your lambda logic into the same Kubernetes cluster as your Dgraph instance. 

{{% notice "tip" %}}
If you want to deploy your own lambda server, you can find the implementation of Dgraph Lambda in our [open-source repository](https://github.com/dgraph-io/dgraph-lambda).
{{% /notice %}}

## Declaring lambda in a GraphQL schema

There are three places where you can use the `@lambda` directive and thus tell Dgraph where to apply custom JavaScript logic.

1. You can add lambda fields to your types and interfaces

```graphql
type MyType {
    ...
    customField: FieldType @lambda
}
```

2. You can add lambda queries to the Query type

```graphql
type Query {
    myCustomQuery(...): QueryResultType @lambda
}
```

3. You can add lambda mutations to the Mutation type

```graphql
type Mutation {
    myCustomMutation(...): MutationResult @lambda
}
```

## Defining lambda resolvers in JavaScript

A lambda resolver is a user-defined JavaScript function that performs custom actions over the GraphQL types, interfaces, queries, and mutations. There are two methods to add a JavaScript resolver:

- `addGraphQLResolvers`
- `addMultiParentGraphQLResolvers`

### addGraphQLResolvers

The `addGraphQLResolvers` method recieves `{ parent, args }` and returns a single value.

- `parent`, the parent object for which to resolve the current lambda field registered using `addGraphQLResolver`.
Available only for types and interfaces (`null` for queries and mutations)
- `args`,  the set of arguments for lambda queries and mutations (`null` for types and interfaces)
- `graphql`, a function to execute auto-generated GraphQL API calls from the lambda server
- `dql`, provides an API to execute DQL from the lambda server


For example:

```javascript
const myTypeResolver = ({parent: {customField}}) => `My value is ${customField}.`

self.addGraphQLResolvers({
    "MyType.customField": myTypeResolver
})
```

Another resolver example using a `graphql` call:

```javascript
async function todoTitles({ graphql }) {
  const results = await graphql('{ queryTodo { title } }')
  return results.data.queryTodo.map(t => t.title)
}

self.addGraphQLResolvers({
  "Query.todoTitles": todoTitles
})
```

### addMultiParentGraphQLResolvers

The `addMultiParentGraphQLResolvers` method receives `{ parents, args }` and return an array of results, each result matching to one parent. 
This method provides a much better performance if you are able to combine multiple requests together.

- `parents`, a list of parent objects for which to resolve the current lambda field registered using `addMultiParentGraphQLResolvers`. Available only for types and interfaces (`null` for queries and mutations)
- `args`,  the set of arguments for lambda queries and mutations (`null` for types/interfaces)
- `graphql`, a function to execute auto-generated GraphQL API calls from the lambda server
- `dql`, provides an API to execute DQL from the lambda server

{{% notice "note" %}}
If the query is a root query or mutation, parents will be set to `[null]`.
{{% /notice %}}

For example:

```javascript
async function rank({parents}) {
    const idRepList = parents.map(function (parent) {
        return {id: parent.id, rep: parent.reputation}
    });
    const idRepMap = {};
    idRepList.sort((a, b) => a.rep > b.rep ? -1 : 1)
        .forEach((a, i) => idRepMap[a.id] = i + 1)
    return parents.map(p => idRepMap[p.id])
}

self.addMultiParentGraphQLResolvers({
    "Author.rank": rank
})
```

Another resolver example using a `dql` call:

```javascript
async function reallyComplexDql({parents, dql}) {
  const ids = parents.map(p => p.id);
  const someComplexResults = await dql.query(`really-complex-query-here with ${ids}`);
  return parents.map(parent => someComplexResults[parent.id])
}

self.addMultiParentGraphQLResolvers({
  "MyType.reallyComplexProperty": reallyComplexDql
})
```

## Example

If you execute this lambda query

```graphql
query {
	queryMyType {
		customField
	}
}
```

You should see a response such as

```json
{
	"queryMyType": [
		{
			"customField":"My value is Lambda Example"
		}
	]
}
```

## Learn more

Find out more about the  `@lambda` directive, or check out:

* [lambda fields](/graphql/lambda/directive)
* [lambda queries](/graphql/lambda/query)
* [lambda mutations](/graphql/lambda/mutation)
* [lambda server setup](/graphql/lambda/server)
