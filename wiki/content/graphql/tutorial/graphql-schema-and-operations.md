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

* type definitions: these define the things in our graph and the shape of the graph.  We'll translate that from the sketch above and in the previous section.
* operations: these define what you can do in the graph through the API, like the search and traversal examples in the previous section.  Initially, Slash GraphQL will generate CRUD operations for us, but, later in the tutorial, we'll move on to defining other operations.

We'll start by translating our design sketch to the SDL and then look at the GraphQL queries and mutations Slash GraphQL generates.

## GraphQL schema

The input schema to Slash GraphQL is a GraphQL schema fragment that contains type definitions.  Slash GraphQL builds a GraphQL API from those definitions.

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

A `type TypeName { ... }` definition defines a kind of node in our graph.  In this case `Post` nodes.  It also gives those nodes what GraphQL calls fields, or data values.  Those fields can be scalar values: in this case a `title`, `text` and `datePublished`;  or links to other nodes: in this case the `author` edge must link to a node of type `User`.

Edges in the graph can be either singular or multiple.  If a field is a name and a type, like `author: User`, then a post can have a single `author` edge.  If a field uses the list notation, like `comments: [Comment]`, then a post can have multiple `comments` edges.

GraphQL allows the schema to mark some fields as required.  For example, we might decide that all users must have a username, but that user might not have set a preferred display name.  If the display name is null, our app can choose to display the username instead.  In GraphQL, that's a `!` annotation on a type.  If the type is set as:

```graphql
type User {
  username: String!
  displayName: String
  ...
}
```

then, our app can be guaranteed that the GraphQL API will never return a user with a null `username`, but might well return a null `displayName`.  This annotation carries over to lists, so `comments: [Comment]` could have both a null list and a list with some nulls in it, while `comments: [Comment!]!` will neither return a null comments list, nor ever return a list with any null values in it.  The `!` helps our UI code make some simplifying assumptions about data the API returns.

### Search

That much GraphQL SDL can be used to describe our types and the shape of our application data graph, and we can start to make a pretty faithful translation of the types in our schema design.  However, there's a bit more that we'll need in the API for this app.  

As well as the shape of the graph, we can use GraphQL directives to tell Slash GraphQL some more about how to interpret the graph and what features we'd like in the GraphQL API.  Slash GraphQL uses this information to specialise the GraphQL API to your needs.

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

A post's `id: ID` gives each post an auto-generated ID.  For users, we'll need a bit more.  The `username` field should be unique; in fact, it should be the id for a user.  Adding the `@id` directive like `username: String! @id` tells Slash GraphQL that username should be a unique id for the user type.  Slash GraphQL will then generate the GraphQL API such that `username` is treated as an id, and ensure that usernames are unique.

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

would specify that an author has some posts and each post has an author, but the schema doesn't connect them as a two-way edge in the graph: e.g. it doesn't say that the posts I can reach from a particular author all have that author as the value of their `author` edge.

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

Working through the four types in the schema sketch, and then adding `@search` and `@hasInverse` directives yields this schema for our app.

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

Some iterations, e.g. adding likes, will just require a schema change and Slash GraphQL will adjust.  Some, e.g. adding an `@search` to comments, can be done by extending the schema and Slash graphQL will index the data and update the API.  Some, e.g. extending the model to include a history of edits on a post, might require us to do a data migration.

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

In GraphQL, the API can be inspected with special queries called introspection queries.  It's just a GraphQL query and is the best way to find out what operations you can perform in a GraphQL API.

### Introspection

Many GraphQL tools support introspection and generate documentation to help you explore an API.  There's any number of tools in the GraphQL ecosystem you can use.  Good choices include [GraphQL Playground](https://github.com/prisma-labs/graphql-playground), [Insomnia](https://insomnia.rest/), [GraphiQL](https://github.com/graphql/graphiql), [Postman](https://www.postman.com/graphql/), and [Altair](https://github.com/imolorhe/altair).

There's also a tool to help you explore your GraphQL API built into Slash GraphQL's Web UI.  Navigate to the "API Explorer" tab and you can access the introspected schema from the right menu.

![Slash GraphQL Schema Explorer](/images/graphql/tutorial/discuss/slash-schema-explorer.png)

From there, you can click through to the queries and mutations and check out the API.  For example, there's mutations to add, update and delete users, posts and comments.

![Slash GraphQL Schema Mutations](/images/graphql/tutorial/discuss/slash-mutations.png)

The best way to learn about what Slash GraphQL built from the schema is to start trying out the queries and mutations we'll be using to build the app.

### Mutations

It'd be great to start by writing some queries to explore our graph, like we did in our sketches exploring graph ideas, but there's no data yet, so we'll start by adding data.

GraphQL mutations have the nice property of returning a result.  That result can help your UI, for example, re-render a page without further queries.  Slash GraphQL let's your post-mutation result, be as expressive as any graph traversal.  Let's start by simply adding a test user.

Our user type has fields `username`, `displayName`, `avatarImg`, `posts` and `comments`

```graphql
type User {
  username: String! @id
  displayName: String
  avatarImg: String
  posts: [Post!]
  comments: [Comment!]
}
```

but the only require field (marked with `!`) is the username.  So to generate a new user node in our graph, we need only to supply a username.  In the GraphQL API, that's an `addUser` mutation.  The mutation can add multiple users at once, but let's just a add one like this:

```graphql
mutation {
  addUser(input: [
    { username: "User1" }
  ]) {
    user {
      username
      displayName
    }
  }
}
```

That adds a single user `User1` and returns the newly created user's `username` and `displayName`.  The `displayName` will be `null` because we didn't enter that data.  There's also no posts, but we aren't asking for that in the result.  Here's how it looks when run in Slash GraphQL's explorer.

![Deploy Slash GraphQL Schema](/images/graphql/tutorial/discuss/slash-add-a-user.png)

Our graph now has a single node, let's also add a category using the `addCategory` mutation.  Categories are a little different to users because the id is auto generated by Slash GraphQL.  This mutation creates the category and returns its `name` and the `id` Slash GraphQL gave it.

```graphql
mutation {
  addCategory(input: [
    { name: "Category1" }
  ]) {
    category {
      id
      name
    }
  }
}
```

Slash GraphQL can do more than add single graph nodes at a time.  The mutations can add whole subgraphs and link into the existing graph however works for your data.  To show this, let's do a few things at once.  Remember our first sketch of some graph data.

![Graph schema sketch](/images/graphql/tutorial/discuss/first-posts-in-graph.png)

At the moment we only have the `User1` and `Category1` nodes.  It's not much of a graph, so let's flesh out the rest of the graph in a single mutation.  We'll use the `addPost` mutation to add the three posts, link all the posts to `User1`, link posts 2 and 3 to the existing category, and, just for good measure, create `Category2`, all in a single mutation. 

```graphql
mutation {
  addPost(input: [
    { 
      title: "Post1",
      text: "Post1",
      author: { username: "User1" },
      category: { 
        name: "Category2" 
      }
    },
    { 
      title: "Post2",
      text: "Post2",
      author: { username: "User1" },
      category: { id: "0x4e22" }
    },
    { 
      title: "Post3",
      text: "Post3",
      author: { username: "User1" },
      category: { id: "0x4e22" }
    }
  ]) {
    post {
      id
      title
      author {
        username
      }
      category {
        name
      }
    }
  }
}
```

Because categories are referenced by an auto-generated `ID`, when you run such a mutation, you'll need to make sure you use the right id value for `Category1`.  In Slash GraphQL's explorer that mutation looked like this:

![Deploy Slash GraphQL Schema](/images/graphql/tutorial/discuss/slash-deep-mutations.png)

Our app probably won't add multiple posts in that way, but it shows the mutation power of Slash GraphQL to for example create a shopping cart and add the first items in a single mutation.  In general, you can serialize your data structures, send them to Slash GraphQL and it mutates the graph, you don't need to programmatically add single objects from the client.

You can run some more mutations, add more users, or posts, or add comments to the posts.  To get you started, here's a mutation that adds some more data that we can use to explore more queries below.

```graphql
mutation {
  addPost(input: [
    { 
      title: "A Post about Slash GraphQL",
      text: "Develop a GraphQL app",
      author: { username: "User1" },
      category: { id: "0x4e22" }
    },
    { 
      title: "A Post about Dgraph",
      text: "It's a GraphQL database",
      author: { username: "User1" },
      category: { id: "0x4e22" }
    },
    { 
      title: "A Post about GraphQL",
      text: "Nice technology for an app",
      author: { username: "User1" },
      category: { id: "0x4e22" }
    },
  ]) {
    post {
      id
      title
    }
  }
}
```

### Queries

GraphQL queries are about starting points and traversals.  A query starts with, for example, `queryPost` or by filtering to some subset of posts like `queryPost(filter: ...)`.  That defines a starting set of nodes in the graph.  From there, your query traverses into the graph and returns the subgraph it finds.  Let's try this out with some examples.

The simplest queries find some nodes and only return data about those nodes, without traversing further into the graph.  The query `queryUser` finds all users.  From those nodes, we can grab out the usernames like this:

```graphql
query {
  queryUser {
    username
  }
}
```

The result will depend on how many users you have added.  If it's just the `User1` sample, then you'll get a result like this:

```json
{
  "data": {
    "queryUser": [
      {
        "username": "User1"
      }
    ]
  }
}
```

That says that the `data` returned is about the `queryUser` query that was executed and here's an array of JSON about those users. 

Because `username` is an identifier, there's also a query that finds users by id.  For example:


```graphql
query {
  getUser(username: "User1") {
    username
  }
}
```

returns:

```json
{
  "data": {
    "getUser": {
      "username": "User1"
    }
  }
}
```

Let's do a bit more traversal into the graph.  In the app's UI, to display, for example, the homepage of a user, we might need to find a user's data and some of their posts.  We saw the beginnings of that sort of traversal in an earlier sketch.

![Graph schema sketch](/images/graphql/tutorial/discuss/user1-post-search-in-graph.png)

As GraphQL, that looks like this:

```graphql
query {
  getUser(username: "User1") {
    username
    displayName
    posts {
      title
    }
  }
}
```

That's: find `User1` as the starting point, grab the `username` and `displayName`, then traverse into the graph following the `posts` edges and get the title of all the posts.

A query could step further into the graph, finding the category of every post, like this:

```graphql
query {
  getUser(username: "User1") {
    username
    displayName
    posts {
      title
      category {
        name
      }
    }
  }
}
```


#### Searching with Filters

To render the app's home screen, we'll need to find a list of posts.  Knowing how to find starting points in the graph and traverse with a query means we can do this much to grab enough data to display a post list.

```graphql
query {
  queryPost {
    id
    title
    author {
      username
    }
    category {
      name
    }
  }
}
```

But we'll also want to limit the number of posts displayed, or order them.  For example, we probably want to limit the number of posts displayed (at least until the user scrolls) and maybe order them newest to oldest.

```graphql
query {
  queryPost(
    order: { desc: datePublished }
    first: 10
  ) {
    id
    title
    author {
      username
    }
    category {
      name
    }
  }
}
```

The UI for our app will also let users search for posts.  That's why we added, for example, `@search(by: [term])` to our schema so that Slash GraphQL would build in an API for searching posts.  The nodes found as the starting points in `queryPost` can be filtered down to match only a subset of posts that have the term "graphql" in the title by adding `filter: { title: { anyofterms: "graphql" }}` to the query.

```graphql
query {
  queryPost(
    filter: { title: { anyofterms: "graphql" }}
    order: { desc: datePublished }
    first: 10
  ) {
    id
    title
    author {
      username
    }
    category {
      name
    }
  }
}
```

The same filtering works during a traversal: find `User1`, and traverse to their posts, but only those posts that have "graphql" in the title.

```graphql
query {
  getUser(username: "User1") {
    username
    displayName
    posts(filter: { title: { anyofterms: "graphql" }}) {
      title
      category {
        name
      }
    }
  }
}
```

## What's next

You can probably start to see how we'll be using the GraphQL queries and mutations as the builing blocks of our UI.  Once a user is logged in, we'll be using `getUser` to grab their preferred display name and avatar; we'll get the list of posts for the main screen with `queryPost`, and maybe add filters, ordering and pagination to that as required; for displaying a single post, we'll use a `getPost` to grab all the required data; a 'new post' button will be hooked up to the `addPost` mutation; and so on.

Next, we'll look at how to start building a UI with Slash GraphQL.  Slash GraphQL generates you a pretty powerful CRUD API from your type definitions.  That means you can get moving on your app idea fast. Later, because GraphQL is really about the API your app needs, you'll eventually need to extend the API, we'll look at how to extend the generated API with more operations.
