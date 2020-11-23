+++
title = "Lambda Mutations"
weight = 4
[menu.main]
    parent = "lambda"
+++

### Schema

To set up a lambda mutation, first you need to define it on your GraphQL schema by using the `@lambda` directive.

For example, to define a lambda mutation for `Author` that creates a new author with a default `reputation` of `3.0` given just the `name`:

```graphql
type Author {
    id: ID!
    name: String! @search(by: [hash, trigram])
    dob: DateTime
    reputation: Float
}

type Mutation {
    newAuthor(name: String!): ID! @lambda
}
```

### Resolver

Once the schema is ready, you can define your JavaScript mutation function and add it as resolver in your JS source code. 
To add the resolver you can use either the `addGraphQLResolvers` or `addMultiParentGraphQLResolvers` methods.

For example, to define the JavaScript `newAuthor()` lambda function and add it as resolver:

```javascript
async function newAuthor({args, graphql}) {
    // lets give every new author a reputation of 3 by default
    const results = await graphql(`mutation ($name: String!) {
        addAuthor(input: [{name: $name, reputation: 3.0 }]) {
            author {
                id
                reputation
            }
        }
    }`, {"name": args.name})
    return results.data.addAuthor.author[0].id
}

self.addGraphQLResolvers({
    "Mutation.newAuthor": newAuthor
})
```

### Example

Finally, if you execute this lambda mutation a new author `Ken Addams` with `reputation=3.0` should be added to the database:

```graphql
mutation {
	newAuthor(name: "Ken Addams")
}
```

Afterwards, if you query the GraphQL database for `Ken Addams`, you would see:

```json
{
	"getAuthor": {
			"name":"Ken Addams",
			"reputation":3.0
		}
}
```
