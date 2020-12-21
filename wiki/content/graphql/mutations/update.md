+++
title = "Update Mutations"
weight = 3
[menu.main]
    parent = "graphql-mutations"
    name = "Update"
+++

Update Mutations allows you to update existing objects of a particular type. It allows to filter nodes and, set and remove any field belonging to a type.

We use the following schema to demonstrate some examples.

**Schema**:
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

Dgraph automatically generates input and return type in the schema for the update mutation. Update mutation takes filter as an input to select specific objects. You can specify set and remove operation on fields belonging to the filtered objects. It returns the state of the objects after updation.
```graphql
updatePost(input: UpdatePostInput!): UpdatePostPayload

input UpdatePostInput {
	filter: PostFilter!
	set: PostPatch
	remove: PostPatch
}

type UpdatePostPayload {
	post(filter: PostFilter, order: PostOrder, first: Int, offset: Int): [Post]
	numUids: Int
}
```

**Example**: Update set mutation using variables
```graphql
mutation updatePost($patch: UpdatePostInput!) {
  updatePost(input: $patch) {
    post {
      postID
      title
      text
    }
  }
}
```
Variables:
```json
{ "patch":
  { "filter": {
      "postID": ["0x123", "0x124"]
    },
    "set": {
      "text": "updated text"
    }
  }
}
```

**Example**: Update remove mutation using variables
```graphql
mutation updatePost($patch: UpdatePostInput!) {
  updatePost(input: $patch) {
    post {
      postID
      title
      text
    }
  }
}
```
Variables:
```json
{ "patch":
  { "filter": {
      "postID": ["0x123", "0x124"]
    },
    "remove": {
      "text": "delete this text"
    }
  }
}
```

## Examples

You can refer to the following [link](https://github.com/dgraph-io/dgraph/blob/master/graphql/resolve/update_mutation_test.yaml) for more examples.
