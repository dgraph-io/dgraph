+++
title = "Deep Mutations"
weight = 5
[menu.main]
    parent = "graphql-mutations"
    name = "Deep"
+++

Mutations also allows to perform deep mutation at multiple levels.

We use the following schema to demonstrate some examples.

## **Schema**:
```graphql
type Author {
	id: ID!
	name: String! @search(by: [hash])
	dob: DateTime
	posts: [Post]
}

type Post {
	postID: ID!
	title: String! @search(by: [term, fulltext])
	text: String @search(by: [fulltext, term])
	datePublished: DateTime
}
```

### **Example**: Deep Deep mutation using variables
```graphql
mutation DeepAuthor($author: DeepAuthorInput!) {
  DeepAuthor(input: [$author]) {
    author {
      id
      name
      post {
        title
        text
      }
    }
  }
}
```
Variables:
```json
{ "author":
  { "name": "A.N. Author",
    "dob": "2000-01-01",
    "posts": [
      {
        "title": "New post",
        "text": "A really new post"
      }
    ]
  }
}
```

### **Example**: Deep update mutation using variables
```graphql
mutation updateAuthor($patch: UpdateAuthorInput!) {
  updateAuthor(input: $patch) {
    author {
      id
      post {
        title
        text
      }
    }
  }
}
```
Variables:
```json
{ "patch":
  { "filter": {
      "id": ["0x123"]
    },
    "set": {
      "posts": [ {
        "postID": "0x456",
        "title": "A new title",
        "text": "Some edited text"
      } ]
    }
  }
}
```
