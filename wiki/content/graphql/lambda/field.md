+++
title = "Lambda Fields"
weight = 2
[menu.main]
    parent = "lambda"
+++

### Schema

To set up a lambda function, first you need to define it on your GraphQL schema by using the `@lambda` directive.

For example, to define a lambda function for the `rank` and `bio` fields in `Author`: 

```graphql
type Author {
        id: ID!
        name: String! @search(by: [hash, trigram])
        dob: DateTime @search
        reputation: Float @search
        bio: String @lambda
        rank: Int @lambda
}
```

You can also define `@lambda` fields on interfaces:

```graphql
interface Character {
        id: ID!
        name: String! @search(by: [exact])
        bio: String @lambda
}

type Human implements Character {
        totalCredits: Float
}

type Droid implements Character {
        primaryFunction: String
}
```

### Resolvers

Once the schema is ready, you can define your JavaScript mutation function and add it as resolver in your JS source code. 
To add the resolver you can use either the `addGraphQLResolvers` or `addMultiParentGraphQLResolvers` methods.

For example, to define JavaScript lambda functions for 
- `Author`, 
- `Character`, 
- `Human`, and 
- `Droid`

and add them as resolvers:

```javascript
const authorBio = ({parent: {name, dob}}) => `My name is ${name} and I was born on ${dob}.`
const characterBio = ({parent: {name}}) => `My name is ${name}.`
const humanBio = ({parent: {name, totalCredits}}) => `My name is ${name}. I have ${totalCredits} credits.`
const droidBio = ({parent: {name, primaryFunction}}) => `My name is ${name}. My primary function is ${primaryFunction}.`

self.addGraphQLResolvers({
    "Author.bio": authorBio,
    "Character.bio": characterBio,
    "Human.bio": humanBio,
    "Droid.bio": droidBio
})
```

Another example, adding a resolver for `rank` using a `graphql` call:

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

### Example

If you execute this lambda query

```graphql
query {
	queryAuthor {
		name
		bio
		rank
	}
}
```

You should see a response such as

```json
{
	"queryAuthor": [
		{
			"name":"Ann Author",
			"bio":"My name is Ann Author and I was born on 2000-01-01T00:00:00Z.",
			"rank":3
		}
	]
}
```

In the same way, if you execute this lambda query on the `Character` interface

```graphql
query {
	queryCharacter {
		name
		bio
	}
}
```

You should see a response such as

```json
{
	"queryCharacter": [
		{
			"name":"Han",
			"bio":"My name is Han."
		},
		{
			"name":"R2-D2",
			"bio":"My name is R2-D2."
		}
	]
}
```

Note that the `Human` and `Droid` types will inherit the `bio` lambda field from the `Character` interface. 

For example, if you execute a `queryHuman` query with a selection set containing `bio`, then the lambda function registered for `Human.bio` will be executed:

```graphql
query {
  queryHuman {
    name
    bio
  }
}
```

Response:

```json
{
  "queryHuman": [
    {
      "name": "Han",
      "bio": "My name is Han. I have 10 credits."
    }
  ]
}
```
