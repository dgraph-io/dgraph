+++
title = "GraphQL Fragements"
weight = 4
[menu.main]
    parent = "api"
    name = "GraphQL Fragements"
+++

A GraphQL fragment is a reusable unit of logic that can be shared between multiple queries and mutations.
Here, we declare a `postData` fragment that can be used with any `Post` object:

```graphql
fragment postData on Post {
  id
  title
  text
  author {
    username
    displayName
  }
}
```

The fragment has a subset of the fields from its associated type. In the above example, the `Post` type must declare all the fields present in the `postData` fragment for it be valid.

We can include the `postData` fragment in any number of queries and mutations as shown below.
```graphql
query allPosts {
  queryPost(order: { desc: title }) {
    ...postData
  }
}
mutation addPost($post: AddPostInput!) {
  addPost(input: [$post]) {
    post {
      ...postData
    }
  }
}
```

The above request is equivalent to:
```graphql
query allPosts {
  queryPost(order: { desc: title }) {
    id
    title
    text
    author {
      username
      displayName
    }
  }
}

mutation addPost($post: AddPostInput!) {
  addPost(input: [$post]) {
    post {
      id
      title
      text
      author {
        username
        displayName
      }
    }
  }
}
```

### Example :

```graphql
fragment postData on Post {
  id
  title
  text
  author {
    username
    displayName
  }
}
mutation addPost($post: AddPostInput!) {
  addPost(input: [$post]) {
    post {
      ...postData
    }
  }
}
```

### Variable:

```graphql
{
	"post": {
    "text": "Hello World",
		"title": "First Blog post",
		"author": [{
			"username": "arijit_ad",
			"displayName": "Arijit Das"
		}]
	}
}
```

### Result:

```graphql
{
  "addPost": {
    "post": [
      {
        "id": "0x27e0",
        "title": "First Blog post",
        "text": "Hello World",
        "author": [
          {
            "username": "arijit_ad",
            "displayName": "Arijit Das"
          }
        ]
      }
    ]
  }
}
```

## Using fragments with interfaces

It is possible to define fragments on interfaces.
Here's an example of a query that includes in-line fragments:

### Schema:

```graphql
interface Employee {
    ename: String!
}
interface Character {
    id: ID!
    name: String! @search(by: [exact])
}
type Human implements Character & Employee {
    totalCredits: Float
}
type Droid implements Character {
    primaryFunction: String
}
```

### Query:

```graphql
query allCharacters {
  queryCharacter {
    name
    __typename
    ... on Human {
      totalCredits
    }
    ... on Droid {
      primaryFunction
    }
  }
}
```

The `allCharacters` query returns a list of `Character` objects. Since `Human` and `Droid` implements the `Character` interface, the fields in the result would be returned according to the type of object.

### Result:

```graphql
{
  "data": {
    "queryCharacter": [
      {
        "name": "Human1",
        "__typename": "Human",
        "totalCredits": 200.23
      },
      {
        "name": "Human2",
        "__typename": "Human",
        "totalCredits": 2.23
      },
      {
        "name": "Droid1",
        "__typename": "Droid",
        "primaryFunction": "Code"
      },
      {
        "name": "Droid2",
        "__typename": "Droid",
        "primaryFunction": "Automate"
      }
    ]
  }
}
```