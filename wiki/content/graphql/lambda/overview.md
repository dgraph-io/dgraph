+++
title = "Overview"
weight = 1
[menu.main]
    parent = "lambda"
    identifier = "lambda-resolvers-overview"
+++

Lambda provides a way to write your custom logic in JavaScript, integrate it with your GraphQL schema, and execute it using the GraphQL API in a few easy steps:

- Setup a Dgraph cluster with a working lambda server (not required for [Slash GraphQL](https://dgraph.io/slash-graphql) users)
- Declare lambda queries, mutations, and fields in your GraphQL schema as needed
- Define lambda resolvers for them in a JavaScript file

This also simplifies the job of developers, as they can build a complex backend that is rich with business logic, without setting up multiple different services. Also, you can build your backend in JavaScript, which means you can build both your frontend and backend using the same language.

Dgraph doesn't execute your custom logic itself. It makes external HTTP requests to a user-defined lambda server. [Slash GraphQL](https://dgraph.io/slash-graphql) will do all of this for you. 

{{% notice "tip" %}}
If you want to deploy your own lambda server, you can find the implementation of Dgraph Lambda in our [open-source repository](https://github.com/dgraph-io/dgraph-lambda). Please refer to the documentation on [setting up a lambda server](/graphql/lambda/server) for more details.
{{% /notice %}}

{{% notice "note" %}}
If you're using [Slash GraphQL](https://dgraph.io/slash-graphql), the final compiled script file must be under 500Kb
{{% /notice %}}

## Declaring lambda in a GraphQL schema

There are three places where you can use the `@lambda` directive and thus tell Dgraph where to apply custom JavaScript logic.

1. You can add lambda fields to your types and interfaces

```graphql
type MyType {
    ...
    customField: String @lambda
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

A lambda resolver is a user-defined JavaScript function that performs custom actions over the GraphQL types, interfaces, queries, and mutations. There are two methods to register JavaScript resolvers:

- `self.addGraphQLResolvers`
- `self.addMultiParentGraphQLResolvers`

### addGraphQLResolvers

The `self.addGraphQLResolvers` method takes an object as an argument, which maps a resolver name to the resolver function that implements it. The resolver functions registered using `self.addGraphQLResolvers` receive `{ parent, args, graphql, dql }` as argument:

- `parent`, the parent object for which to resolve the current lambda field registered using `addGraphQLResolver`.
The `parent` receives all immediate fields of that object, whether or not they were actually queried.
Available only for types and interfaces (`null` for queries and mutations)
- `args`,  the set of arguments for lambda queries and mutations
- `graphql`, a function to execute auto-generated GraphQL API calls from the lambda server. The user's auth header is passed back to the `graphql` function, so this can be used securely
- `dql`, provides an API to execute DQL from the lambda server

{{% notice "tip" %}}
`self.addGraphQLResolvers` is the default choice for registering resolvers when the result of the lambda for each parent is independant of other parents.
{{% /notice %}}

Each resolver function should return data in the exact format as the return type of GraphQL field, query, or mutation for which it is being registered.

In the following example, the resolver function `myTypeResolver` registered for the `customField` field in `MyType` returns a string because the return type of that field in the GraphQL schema is `String`:

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

The `self.addMultiParentGraphQLResolvers` is useful in scenarios where you want to perform computations involving all the parents returned from Dgraph for a lambda field. This is useful in two scenarios:

- When you want to perform a computation between parents
- When you want to execute a complex query, and want to optimize it by firing a single query for all the parents

This method takes an object as an argument, which maps a resolver name to the resolver function that implements it. The resolver functions registered using this method receive `{ parents, args, graphql, dql }` as argument:

- `parents`, a list of parent objects for which to resolve the current lambda field registered using `addMultiParentGraphQLResolvers`. Available only for types and interfaces (`null` for queries and mutations)
- `args`,  the set of arguments for lambda queries and mutations (`null` for types and interfaces)
- `graphql`, a function to execute auto-generated GraphQL API calls from the lambda server
- `dql`, provides an API to execute DQL from the lambda server

{{% notice "note" %}}
This method should not be used for lambda queries or lambda mutations.
{{% /notice %}}

Each resolver function should return data as a list of the return type of GraphQL field for which it is being registered. 

In the following example, the resolver function `rank()` registered for the `rank` field in `Author`, returns a list of integers because the return type of that field in the GraphQL schema is `Int`:

```graphql
type Author {
    id: ID!
    name: String! @search(by: [hash, trigram])
    reputation: Float @search
    rank: Int @lambda
}
```

```javascript
import { sortBy } from 'lodash';

/* 
This function computes the rank of each author based on the reputation of the author relative to other authors.
*/
async function rank({parents}) {
    const idRepMap = {};
    _.sortBy(parents, 'reputation').forEach((parent, i) => idRepMap[parent.id] = parents.length - i)
    return parents.map(p => idRepMap[p.id])
}

self.addMultiParentGraphQLResolvers({
    "Author.rank": rank
})
```

{{% notice "note" %}}
Webpack is required to use the sample `rank()` function.
{{% /notice %}}

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
