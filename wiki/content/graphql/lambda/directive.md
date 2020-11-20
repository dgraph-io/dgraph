+++
title = "The @lambda directive"
weight = 2
[menu.main]
    parent = "lambda"
+++

The `@lambda` directive in GraphQL allows you to call custom JavaScript resolvers. All the `@lambda` fields are resolved through the lambda functions implemented on a user-defined lambda server.

## Schema

To set up a lambda function, first you need to define it on your GraphQL schema by using the `@lambda` directive.

For example, to define a lambda function for the `rank` and `bio` fields in `Author`: 

```graphql
type Author {
        id: ID!
        name: String! @search(by: [hash, trigram])

        dob: DateTime @search
        reputation: Float @search
        qualification: String @search(by: [hash, trigram])
        country: Country
        posts: [Post!] @hasInverse(field: author)
        bio: String @lambda
        rank: Int @lambda
}
```

You can also define `@lambda` fields on interfaces:

```graphql
interface Character {
        id: ID!
        name: String! @search(by: [exact])
        appearsIn: [Episode!] @search
        bio: String @lambda
}

type Human implements Character {
        starships: [Starship]
        totalCredits: Float
}

type Droid implements Character {
        primaryFunction: String
}
```

## Resolvers

A lambda resolver is a user-defined JavaScript function that performs custom actions over the GraphQL types, interfaces, queries, and mutations. Any lambda resolver can receive up to 4 parameters:
- `parent`, a reference to the parent type
- `args`, a set of function arguments
- `graphql`, a custom GraphQL query
- `dql`, a custom DQL query

There are two methods to add a JavaScript resolver:
- `addGraphQLResolvers`
- `addMultiParentGraphQLResolvers`

### addGraphQLResolvers

The `addGraphQLResolvers` method recieves `{ parent, args }` and returns a single value.

For example:

```javascript
const authorBio = ({parent: {name, dob}}) => `My name is ${name} and I was born on ${dob}.`
const characterBio = ({parent: {name}}) => `My name is ${name}.`
const humanBio = ({parent: {name, totalCredits}}) => `My name is ${name}. I have ${totalCredits} credits.`
const droidBio = ({parent: {name, primaryFunction}}) => `My name is ${name}. My primary function is ${primaryFunction}.`

self.addGraphQLResolvers({
    "Author.bio": authorBio,
    "Character.bio": characterBio,
    "Human.bio": humanBio,
    "Droid.bio": droidBio,
    "Query.authorsByName": authorsByName,
    "Mutation.newAuthor": newAuthor
})
```

Another resolver example using a `graphql` call:

```javascript
const fullName = ({ parent: { firstName, lastName } }) => `${firstName} ${lastName}`

async function todoTitles({ graphql }) {
  const results = await graphql('{ queryTodo { title } }')
  return results.data.queryTodo.map(t => t.title)
}

self.addGraphQLResolvers({
  "User.fullName": fullName,
  "Query.todoTitles": todoTitles,
})
```

### addMultiParentGraphQLResolvers

The `addMultiParentGraphQLResolvers` method receives `{ parents, args }` and return an array of results, each result matching to one parent. 
This method provides a much better performance if you are able to combine multiple requests together.

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
  const someComplexResults = await dql(`really-complex-query-here with ${ids}`);
  return parents.map(parent => someComplexResults[parent.id])
}

self.addMultiParentGraphQLResolvers({
  "User.reallyComplexProperty": reallyComplexDql
})
```

## Learn more

Find out more about how to apply lambdas to GraphQL mutations and queries at:

- [Lambda queries](/graphql/lambda/query)
- [Lambda mutations](/graphql/lambda/mutation)
