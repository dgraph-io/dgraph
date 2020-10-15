+++
title = "IDs"
weight = 3
[menu.main]
    parent = "schema"
+++

There's two types of identity built into Dgraph.  Those are accessed via the `ID` scalar type and the `@id` directive.

### The ID type

In Dgraph, every node has a unique 64 bit identifier.  You can, but don't have to, expose that in GraphQL via the `ID` type.  `ID`s are auto-generated, immutable and never reused.  Each type can have at most one `ID` field.

The `ID` type works great for things that you'll want to refer to via an id, but don't need to set the identifier externally.  Examples are things like posts, comments, tweets, etc. 

For example, you might set the following type in a schema.

```graphql
type Post {
    id: ID!
    ...
}
```

In a single page app, you'll want to render the page for `http://.../posts/0x123` when a user clicks to view the post with id `0x123`.  You app can then use a `getPost(id: "0x123") { ... }` GraphQL query to fetch the data to generate the page.

You'll also be able to update and delete posts by id.

### The @id directive

For some types, you'll need a unique identifier set from outside Dgraph.  A common example is a username.

The `@id` directive tells Dgraph to keep values of that field unique and to use them as identifiers.

For example, you might set the following type in a schema.

```graphql
type User {
    username: String! @id
    ...
}
```

Dgraph will then require a unique username when creating a new user --- it'll generate the input type for `addUser` with `username: String!` so you can't make an add mutation without setting a username, and when processing the mutation, Dgraph will ensure that the username isn't already set for another node of the `User` type.

Identities created with `@id` are reusable - if you delete an existing user, you can reuse the username.

As with `ID` types, Dgraph will generate queries and mutations so you'll also be able to query, update and delete by id.

### More to come

We are currently considering expanding uniqueness to include composite ids and multiple unique fields (e.g. [this](https://discuss.dgraph.io/t/support-multiple-unique-fields-in-dgraph-graphql/8512) issue).
