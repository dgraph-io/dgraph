+++
title = "Lambda Queries"
weight = 3
[menu.main]
    parent = "lambda"
+++

### Schema

To set up a lambda query, first you need to define it on your GraphQL schema by using the `@lambda` directive.

For example, to define a lambda query for `Author` that finds out authors given an author's `name`:

```graphql
type Author {
    id: ID!
    name: String! @search(by: [hash, trigram])
    dob: DateTime
    reputation: Float
}

type Query {
    authorsByName(name: String!): [Author] @lambda
}
```

### Resolver

Once the schema is ready, you can define your JavaScript query function and add it as resolver in your JS source code. 
To add the resolver you can use either the `addGraphQLResolvers` or `addMultiParentGraphQLResolvers` methods.

For example, to define the JavaScript `authorsByName()` lambda function and add it as resolver:

```javascript
async function authorsByName({args, dql}) {
    const results = await dql.query(`query queryAuthor($name: string) {
        queryAuthor(func: type(Author)) @filter(eq(Author.name, $name)) {
            name: Author.name
            dob: Author.dob
            reputation: Author.reputation
        }
    }`, {"$name": args.name})
    return results.data.queryAuthor
}

self.addGraphQLResolvers({
    "Query.authorsByName": authorsByName,
})
```

### Example

Finally, if you execute this lambda query

```graphql
query {
	authorsByName(name: "Ann Author") {
		name
		dob
		reputation
	}
}
```

You should see a response such as

```json
{
	"authorsByName": [
		{
			"name":"Ann Author",
			"dob":"2000-01-01T00:00:00Z",
			"reputation":6.6
		}
	]
}
```
