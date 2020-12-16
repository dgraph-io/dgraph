+++
title = "Deep Mutations"
weight = 5
[menu.main]
    parent = "graphql-mutations"
    name = "Deep"
+++

Mutations also allows to perform deep mutation at multiple levels. Deep mutations do not alter linked objects, but can add deeply nested new or existing objects. In the case where you wish to update a nested existing object, you will need to use the update mutation for it's type.

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

### **Example**: Add new nested object
```graphql
mutation updateAuthorWithNewPost($author: DeepAuthorInput!) {
  updateAuthor(input: [$author]) {
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

### **Example**: Link existing nested object

The following assumes that the post with the postID of `0x456` already exists, and is not currently nested under the author having the id of `0x123`.

Note: this syntax does not remove any other existing posts, but rather adds the existing post to any that may already be nested.

```graphql
mutation updateAuthorWithExitingPost($patch: UpdateAuthorInput!) {
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
      "posts": [
        {
          "postID": "0x456"
        } 
      ]
    }
  }
}
```

In this example above, we could not have effectively modified the existing post's title or text in this query. If we need to modify the post's title or text, we would need to use the `updatePost` mutation either along side the mutation above or as a separate transaction.
