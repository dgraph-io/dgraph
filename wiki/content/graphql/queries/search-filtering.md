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

Fetch the `title`, `text` and `datePublished` for a post with id `0x1`.

```graphql
query {
  getPost(id: "0x1") {
    title
    text
    datePublished
  }
}
```

Fetching nested linked objects, while using `get` queries is also easy. For example, this is how you would fetch the authors for a post and their friends.

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

For example, the following query fetches the author with the `id` 0x1 and their
 posts about `GraphQL`.

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

If your type has a field with the `@id` directive applied to it, you can also fetch objects using that.

For example, given the following schema, the query below fetches a user's `name` and `age` by `userID` (which has the `@id` directive):

**Schema**:

```graphql
type User {
    userID: String! @id
    name: String!
    age: String
}
```

**Query**:

```graphql
query {
  getUser(userID: "0x2") {
    name
    age
  }
}
```

### Query a list of objects

You can query a list of objects using GraphQL. For example, the following query fetches the `title`, `text` and and `datePublished` for all posts:

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

The following example query fetches a list of posts by their post `id`:

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

You also filter posts by different fields in the `Post` type that have a `@search` directive applied. To only fetch posts which have `GraphQL` in their title and have a `score > 100`, you can run the following query:

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

### Filter a query for a list of objects

You can also filter nested objects while querying for a list of objects.

For example, the following query fetches all of the authors whose name contains `Lee` and with their `completed` posts that have a score greater than `10`:

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

### Filter a query for a range of objects with `between`

You can also filter query results within an inclusive range of indexed and typed
scalar values using the `between` keyword.

{{% notice "tip" %}}This keyword is also supported for DQL; to learn more, see
[DQL Functions: `between`](/query-language/functions/#between).{{% /notice %}}


For example, you might start with the following example schema used to track
students at a school:

**Schema**:

```graphql
type Student{
   age: Int @search
   name: String @search(by: [exact])
}
```
Using the `between` filter, you could fetch records for students who are between
10 and 20 years of age:

**Query**:

```graphql
queryStudent(fitler: {age: between: {min: 10, max: 20}}){
    age
    name
}
```

You could also use this filter to fetch records for students whose names fall
alphabetically between `ba` and `hz`:

**Query**:

```graphql
queryStudent(fitler: {name: between: {min: "ba", max: "hz"}}){
    age
    name
}
```
