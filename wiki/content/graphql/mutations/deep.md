+++
title = "Deep Mutations"
weight = 5
[menu.main]
    parent = "graphql-mutations"
    name = "Deep"
+++

Mutations also allows to perform deep mutation at multiple levels. Deep mutations do not alter linked objects, but they can add deeply-nested new or existing objects. To update an existing nested object, use the update mutation for its type.

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

The following example assumes that the post with the postID of `0x456` already exists, and is not currently nested under the author having the id of `0x123`.

{{% notice "note" %}}
This syntax does not remove any other existing posts, it just adds the existing post to any that may already be nested.
{{% /notice %}}

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

The example query above can't modify the existing post's title or text. To modify the post's title or text, use the `updatePost` mutation either alongside the mutation above, or as a separate transaction.
