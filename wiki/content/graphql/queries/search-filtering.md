+++
title = "Search and Filtering"
weight = 2
[menu.main]
    parent = "graphql-queries"
    name = "Search and Filtering"
+++

Queries generated for a GraphQL type allow you to generate a single of list of
objects for a type.

### Get a single object

Fetch the title, text and datePublished for a post with id `0x1`.

```graphql
query {
  getPost(id: "0x1") {
    title
    text
    datePublished
  }
}
```

Fetching nested linked objects, while using get queries is also easy. This is how
you would fetch the authors for a post and their friends.

```graphql
query {
  getPost(id: "0x1") {
    id
    title
    text
    datePublished
    author {
      name
      friends {
        name
      }
    }
  }
}
```

While fetching nested linked objects, you can also apply a filter on them.

Example - Fetching author with id 0x1 and their posts about GraphQL.

```graphql
query {
  getAuthor(id: "0x1") {
    name
    posts(filter: {
      title: {
        allofterms: "GraphQL"
      }
    }) {
      title
      text
      datePublished
    }
  }
}
```

If your type has a field with `@id` directive on it, you can also fetch objects using that.

Example: To fetch a user's name and age by userID which has @id directive.

Schema

```graphql
type User {
    userID: String! @id
    name: String!
    age: String
}
```

Query

```graphql
query {
  getUser(userID: "0x2") {
    name
    age
  }
}
```

### Query list of objects

Fetch the title, text and and datePublished for all the posts.

```graphql
query {
  queryPost {
    id
    title
    text
    datePublished
  }
}
```

Fetching a list of posts by their ids.

```graphql
query {
  queryPost(filter: {
    id: ["0x1", "0x2", "0x3", "0x4"],
  }) {
    id
    title
    text
    datePublished
  }
}
```

You also filter the posts by different fields in the Post type which have a
`@search` directive on them. To only fetch posts which `GraphQL` in their title
and have a `score > 100`, you can run the following query.

```graphql
query {
  queryPost(filter: {
    title: {
      anyofterms: "GraphQL"
    },
    and: {
      score: {
        gt: 100
      }
    }
  }) {
    id
    title
    text
    datePublished
  }
}
```

You can also filter nested objects while querying for a list of objects.

Example - To fetch all the authors whose name have `Lee` in them and their`completed` posts
with score greater than 10.

```graphql
query {
  queryAuthor(filter: {
    name: {
      anyofterms: "Lee"
    }
  }) {
    name
    posts(filter: {
      score: {
        gt: 10
      },
      and: {
        completed: true
      }
    }) {
      title
      text
      datePublished
    }
  }
}
```