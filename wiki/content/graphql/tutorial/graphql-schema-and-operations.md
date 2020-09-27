+++
title = "GraphQL"
[menu.main]
    parent = "build-an-app-tutorial"
    identifier = "graphql-schema-and-operations"
    weight = 4   
+++

*In this section: you'll learn about how to translate the schema design to the GraphQL SDL (Schema Definition Language) and how to run GraphQL queries and mutations.*

In the schema design section, we came up with this sketch of our graph schema.

![Graph schema sketch](/images/graphql/tutorial/discuss/schema-sketch.png)

Slash GraphQL generates a running GraphQL API from the description of a schema as GraphQL types, using the GraphQL SDL.  We'll start by translating our design to the SDL and then look at GraphQL queries and mutations

## GraphQL schema

A GraphQL schema contains type definitions and operations (queries and mutations) that can be performed on those types.

### Types

There's some pre-defined scalar types (`ID`, `Int`, `String`, `Float` and `DateTime`) and a schema can define any number of other types.  For example, from our schema sketch we can start to define the `Post` type as follows.

```graphql
type Post {
  id: ID
  title: String
  text: String
  datePublished: DateTime
  author: User
  ...
}
```

A `type TypeName { ... }` definition defines a kind of node in our graph --- in this case `Post` nodes.  It also gives those nodes what GraphQL calls fields, data values.  Those can be scalar values --- in this case an `id`, `title`, `text` and `datePublished` --  or links to other nodes --- in this case the `author` edge must link to a node of type `User`.

Edges in the graph can be either singular or multiple.  If a field is a name and a type, like `author: User`, then a post can have a single `author` edge.  If a field uses the list notation, like `comments: [Comment]`, then a post can have multiple `comments` edges.

GraphQL allows the schema to mark some fields as required.  For example, we might decide that all users must have a username, but that user might not have set a preferred display name.  If the display name is null, our app will display the username instead.  In GraphQL, that's a `!` annotation on a type.  If the type is set as:

```graphql
type User {
  username: String!
  displayName: String
  ...
}
```

then, our app can be guaranteed that the GraphQL API will never return a user with a null `username`, but might well return a null `displayName`.  This annotation carries over to lists, so `comments: [Comment]` could have both a null list and a list with some nulls in it, while `comments: [Comment!]!` will neither return a null comments list, nor ever return a list with any null values in it.  This helps our UI code make some simplifying assumptions about data the API returns.

### Search

That much GraphQL SDL can be used to describe our types and the shape of our application data graph.  However, there's a bit more that we'll need for this app.  As well as the shape of the graph, we can tell Slash GraphQL some more about how to interpret the graph using GraphQL directives.  Slash GraphQL uses this information to specialise the GraphQL API to your needs.

For example, with just the type definition, Slash GraphQL doesn't know what kinds of search we'll need built into our API.  The schema tells Slash GraphQL about the search needed with the `@search` directive.  Adding search directives like this:

```graphql
type Post {
  ...
  title: String! @search(by: [term])
  text: String! @search(by: [fulltext])
  ...
}
```

tells Slash GraphQL that we want it to generate an API that allows for search for posts by title using terms and post text by fulltext search.  That'd be search like 'all the posts with `GraphQL` in the title' and post text Goolge-style search like 'all the posts about `developing GraphQL apps`.

### IDs

A post's `id: ID` gives each post an auto-generated 64-bit id.  For users, we'll want a bit more.  The `username` field should be unique; in fact, it should be the id for a user.  Adding the `@id` directive like `username: String! @id` tells Slash GraphQL that username should be a unique id for the user type.  It will then generate the GraphQL API such that `username` is treated as an id, and ensure that usernames are unique.

### Relationships

The only remaining thing is to recognize how GraphQL handles relations. So far, our GraphQL schema says that an author has some questions and answers and that a post has an author, but the schema doesn't connect them as a two-way edge in the graph: e.g. it doesn't say that the questions I can reach from a particular author all have that author as their author.

GraphQL schemas are always under-specified in this way. It's left up to the documentation and implementation to make the two-way connection, if it exists. Here, we'll make sure they hook up in the right way by adding the directive @hasInverse.


### Final schema

taken the drawing and translated the fields and relationships ... and then added some search that looks interesting (`@id` automatically generates search by username)

```graphql
type User {
  username: String! @id
  displayName: String
  avatarImg: String
  posts: [Post] @hasInverse(field: author)
}

type Post {
  id: ID!
  title: String! @search(by: [term])
  text: String! @search(by: [fulltext])
  datePublished: DateTime
  author: User!
  category: Category! @hasInverse(field: posts)
  comments: [Comment!]!
}

type Comment {
  id: ID!
  text: String!
  commentsOn: Post! @hasInverse(field: comments)
  author: User!
}

type Category {
  id: ID!
  name: String! @search(by: [term])
  posts: [Post]
}
```

## Slash GraphQL

...adding the schema to slash

## GraphQL Operations

Schema and operations... follow on from our design

- in GraphQL it'll be operations that that show the valid things you can do on the data graph

### mutations


```graphql
mutation {
  addUser(input: [
    {
      username: "alice@dgraph.io",
      name: "Alice",
      tasks: [
        {
          title: "Avoid touching your face",
          completed: false,
        },
        {
          title: "Stay safe",
          completed: false
        },
        {
          title: "Avoid crowd",
          completed: true,
        },
        {
          title: "Wash your hands often",
          completed: true
        }
      ]
    }
  ]) {
    user {
      username
      name
      tasks {
        id
        title
      }
    }
  }
}
```

### queries

```graphql
query {
  queryTask {
    id
    title
    completed
    user {
        username
    }
  }
}
```

Running the query above should return JSON response as shown below:

```json
{
  "data": {
    "queryTask": [
      {
        "id": "0x3",
        "title": "Avoid touching your face",
        "completed": false,
        "user": {
          "username": "alice@dgraph.io"
        }
      },
      {
        "id": "0x4",
        "title": "Stay safe",
        "completed": false,
        "user": {
          "username": "alice@dgraph.io"
        }
      },
      {
        "id": "0x5",
        "title": "Avoid crowd",
        "completed": true,
        "user": {
          "username": "alice@dgraph.io"
        }
      },
      {
        "id": "0x6",
        "title": "Wash your hands often",
        "completed": true,
        "user": {
          "username": "alice@dgraph.io"
        }
      }
    ]
  }
}
```


#### Querying Data with Filters

Before we get into querying data with filters, we will be required
to define search indexes to the specific fields.

Let's say we have to run a query on the _completed_ field, for which
we add `@search` directive to the field, as shown in the schema below:

```graphql
type Task {
  id: ID!
  title: String!
  completed: Boolean! @search
  user: User!
}
```

The `@search` directive is added to support the native search indexes of **Dgraph**.

Resubmit the updated schema -
```
curl -X POST localhost:8080/admin/schema --data-binary '@schema.graphql'
```

Now, let's fetch all todos which are completed :

```graphql
query {
  queryTask(filter: {
    completed: true
  }) {
    title
    completed
  }
}
```

Next, let's say we have to run a query on the _title_ field, for which
we add another `@search` directive to the field, as shown in the schema below:

```graphql
type Task {
    id: ID!
    title: String! @search(by: [fulltext])
    completed: Boolean! @search
    user: User!
}
```

The `fulltext` search index provides the advanced search capability to perform equality
comparison as well as matching with language-specific stemming and stopwords.

Resubmit the updated schema -
```
curl -X POST localhost:8080/admin/schema --data-binary '@schema.graphql'
```

Now, let's try to fetch todos whose title has the word _"avoid"_ :

```graphql
query {
  queryTask(filter: {
    title: {
      alloftext: "avoid"
    }
  }) {
    id
    title
    completed
  }
}
```


### subscriptions

## What's next
