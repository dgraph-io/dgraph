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

Slash GraphQL generates a running GraphQL API from the description of a schema as GraphQL types, using the GraphQL SDL (Schema Definition Language).  There's really two separate things in a GraphQL schema: 

* type definitions: these define the things in our graph and the shape of the graph, just like our sketches in the previous section.
* operations: these define what you can do in the graph through the API, like the search and traversal examples in the previous section.  

We'll start by translating our design sketch to the SDL and then look at the GraphQL queries and mutations Slash GraphQL generates.

## GraphQL schema

A GraphQL schema contains type definitions and operations (queries and mutations) that can be performed on those types.

### Types

There's some pre-defined scalar types (`Int`, `String`, `Float` and `DateTime`) and a schema can define any number of other types.  For example, we can start to define the `Post` type in the GraphQL SDL by translating from our sketch as follows.

```graphql
type Post {
  title: String
  text: String
  datePublished: DateTime
  author: User
  ...
}
```

A `type TypeName { ... }` definition defines a kind of node in our graph --- in this case `Post` nodes.  It also gives those nodes what GraphQL calls fields, or data values.  Those fields can be scalar values --- in this case a `title`, `text` and `datePublished` --  or links to other nodes --- in this case the `author` edge must link to a node of type `User`.

Edges in the graph can be either singular or multiple.  If a field is a name and a type, like `author: User`, then a post can have a single `author` edge.  If a field uses the list notation, like `comments: [Comment]`, then a post can have multiple `comments` edges.

GraphQL allows the schema to mark some fields as required.  For example, we might decide that all users must have a username, but that user might not have set a preferred display name.  If the display name is null, our app can choose to display the username instead.  In GraphQL, that's a `!` annotation on a type.  If the type is set as:

```graphql
type User {
  username: String!
  displayName: String
  ...
}
```

then, our app can be guaranteed that the GraphQL API will never return a user with a null `username`, but might well return a null `displayName`.  This annotation carries over to lists, so `comments: [Comment]` could have both a null list and a list with some nulls in it, while `comments: [Comment!]!` will neither return a null comments list, nor ever return a list with any null values in it.  This helps our UI code make some simplifying assumptions about data the API returns.

### Search

That much GraphQL SDL can be used to describe our types and the shape of our application data graph, and we can start to make a pretty faithful translation of the types in our schema design.  However, there's a bit more that we'll need in the API for this app.  

As well as the shape of the graph, we can use GraphQL directives tell Slash GraphQL some more about how to interpret the graph and what features we'd like in the GraphQL API.  Slash GraphQL uses this information to specialise the GraphQL API to your needs.

For example, with just the type definition, Slash GraphQL doesn't know what kinds of search we'll need built into our API.  Adding the `@search` directive to the schema tells Slash GraphQL about the search needed.  Adding search directives like this:

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

There's two types of identifiers in Slash GraphQL: an `ID` type that gives auto-generated 64-bit IDs, and an `@id` directive that allows external IDs to be used.  The two have different purposes. For example, `ID` is best for things like posts that need a unique id, but our app doesn't care what it is, while `@id` is best for types like users where we care that the id, their username, is supplied by them and unique.

```graphql
type Post {
  id: ID!
  ...
}

type User {
  username: String! @id
  ...
}
```

A post's `id: ID` gives each post an auto-generated ID.  For users, we'll ne a bit more.  The `username` field should be unique; in fact, it should be the id for a user.  Adding the `@id` directive like `username: String! @id` tells Slash GraphQL that username should be a unique id for the user type.  Slash GraphQL will then generate the GraphQL API such that `username` is treated as an id, and ensure that usernames are unique.

### Relationships

The only remaining thing is to recognize how GraphQL handles relations. A GraphQL schema based around types like 

```graphql
type User {
  ...
  posts: [Post!]!
}

type Post {
  ...
  author: User!
}
```

would specify that an author has some posts and each post has an author, but the schema doesn't connect them as a two-way edge in the graph: e.g. it doesn't say that the posts I can reach from a particular author all have that author as their author.

GraphQL schemas are always under-specified in this way. It's left up to the documentation and implementation to make the two-way connection, if it exists. There might be multiple connections between two types; for example, an author might also be linked the the posts they have commented on.  So it makes sense that we need something other than just the types to connect all the edges.

In Slash GraphQL, we can specify two-way edges by adding the `@hasInverse` directive.  That lets us untangle situations where there's multiple relationships, so Slash GraphQL can do the right bookkeeping for us.  In our app, for example, we might need to make sure that the relationships between the posts that a user has authored and the ones they've liked linked correctly.

```graphql
type User {
  ...
  posts: [Post!]! 
  liked: [Post!]!
}

type Post {
  ...
  author: User! @hasInverse(field: posts)
  likedBy: [User!]! @hasInverse(field: liked)
}
```

The `@hasInverse` directive is only needed on one end of a two-way edge, but you can add it at both ends if that works for your documentation purposes.

### Final schema

Working through the four types in the schema sketch, and then adding `@search` directives yields this schema for our app.

```graphql
type User {
  username: String! @id
  displayName: String
  avatarImg: String
  posts: [Post!]
  comments: [Comment!]
}

type Post {
  id: ID!
  title: String! @search(by: [term])
  text: String! @search(by: [fulltext])
  datePublished: DateTime
  author: User!  @hasInverse(field: posts)
  category: Category! @hasInverse(field: posts)
  comments: [Comment!]
}

type Comment {
  id: ID!
  text: String!
  commentsOn: Post! @hasInverse(field: comments)
  author: User! @hasInverse(field: comments)
}

type Category {
  id: ID!
  name: String! @search(by: [term])
  posts: [Post!]
}
```

Slash GraphQL is built to allow for iteration on our schema.  I'm sure you've picked up things that could be added to enhance our app, for example, adding up and down votes or likes to the posts.  We'll leave those to later sections in the tutorial and build the app like you would in working on a project of your own: start by building a working minimal version and then iterate from there.

Some iterations, e.g. adding likes, will just require a schema change and Slash GraphQL will adjust.  Some, e.g. adding an `@search` to comments, can be done by extending the schema and Slash graphQL will index the data and update the API.  Some, e.g. extending the model to include a history of edits on a post, might require more complex data migrations.

## Slash GraphQL

With the schema defined, it's just one step to get a running GraphQL backend for our app.

Copy the schema, navigate to the 'Schema' tab in Slash GraphQL, paste the schema in, and press 'Deploy'

![Deploy Slash GraphQL Schema](/images/graphql/tutorial/discuss/slash-deploy-schema.png)

As soon as the schema is added, Slash GraphQL has deployed a GraphQL API for our app.  Let's look at how our ideas about graphs and traversals translate into GraphQL.

## GraphQL Operations

The schema we developed was about the types in our domain and the shape of the application data graph.  In GraphQL, the things we can do in the graph are called operations.  Those operations are either:

* queries --- find a starting point and traverse a subgraph,
* mutations --- change the graph and return a result, or
* subscriptions --- listen to changes in the graph.

In GraphQL, the API can be inspected with special queries called introspection queries.  That's the best way to find out what operations you can perform in a GraphQL API.

### introspection


![Deploy Slash GraphQL Schema](/images/graphql/tutorial/discuss/slash-schema-explorer.png)

![Deploy Slash GraphQL Schema](/images/graphql/tutorial/discuss/slash-mutations.png)

### mutations


![Deploy Slash GraphQL Schema](/images/graphql/tutorial/discuss/slash-add-a-user.png)

![Deploy Slash GraphQL Schema](/images/graphql/tutorial/discuss/slash-deep-mutations.png)

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


Slash GraphQL generates you a pretty powerful CRUD API from your type definitions, but, because GraphQL is really about the API your app needs, you'll eventually needs ... can do more ... look at that in future sections