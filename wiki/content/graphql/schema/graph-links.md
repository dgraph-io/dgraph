+++
title = "Links in the Graph"
weight = 4
[menu.main]
    parent = "schema"
+++

All the data in your app forms a GraphQL data graph.  That graph has nodes of particular types (the types you define in your schema) and links between the nodes to form the data graph.

Dgraph uses the types and fields in the schema to work out how to link that graph, what to accept for mutations and what shape responses should take.  

Edges in that graph are directed: either pointing in one direction or two.  You use the `@hasInverse` directive to tell Dgraph how to handle two-way edges.

### One-way Edges

If you only ever need to traverse the graph between nodes in a particular direction, then your schema can simply contain the types and the link. 

In this schema, posts have an author - each post in the graph is linked to its author - but that edge is one-way.  

```graphql
type Author {
    ...
}

type Post {
    ...
    author: Author
}
```

You'll be able to traverse the graph from a Post to its author, but not able to traverse from an author to all their posts.  Sometimes that's the right choice, but mostly, you'll want two way edges.  

Note: Dgraph won't store the reverse direction, so if you change your schema to include a `@hasInverse`, you'll need to migrate the data to add the reverse edges.

### Two-way edges - edges with an inverse

GraphQL schemas are always under-specified in that if we extended our schema to:

```graphql
type Author {
    ...
    posts: [Post]
}

type Post {
    ...
    author: Author
}
```

Then, the schema says that an author has a list of posts and a post has an author.  But, that GraphQL schema doesn't say that every post in the list of posts for an author has the same author as their `author`.  For example, it's perfectly valid for author `a1` to have a `posts` edge to post `p1`, that has an `author` edge to author `a2`.  Here, we'd expect an author to be the author of all their posts, but that's not what GraphQL enforces.  In GraphQL, it's left up to the implementation to make two-way connections in cases that make sense.  That's just part of how GraphQL works.

In Dgraph, the directive `@hasInverse` is used to create a two-way edge.  

```graphql
type Author {
    ...
    posts: [Post] @hasInverse(field: author)
}

type Post {
    ...
    author: Author
}
```

With that, `posts` and `author` are just two directions of the same link in the graph.  For example,  adding a new post with

```graphql
mutation {
    addPost(input: [ 
        { ..., author: { username: "diggy" }}
    ]) {
        ...
    }
}
```

will automatically add it to Diggy's list of `posts`.  Deleting the post will remove it from Diggy's `posts`.  Similarly, using an update mutation on an author to insert a new post will automatically add Diggy as the author the author

```graphql
mutation {
    updateAuthor(input: {
        filter: { username: { eq: "diggy "}},
        set: { posts: [ {... new post ...}]}
    }) {
        ...
    }
}
```

### Many edges

It's not really possible to auto-detect what a schema designer meant for two-way edges.  There's not even only one possible relationship between two types. Consider, for example, if an app recorded the posts an `Author` had recently liked (so it can suggest interesting material) and just a tally of all likes on a post.

```graphql
type Author {
    ...
    posts: [Post]
    recentlyLiked: [Post]
}

type Post {
    ...
    author: Author
    numLikes: Int
}
```

It's not possible to detect what is meant here as a one-way edge, or which edges are linked as a two-way connection.  That's why `@hasInverse` is needed - so you can enforce the semantics your app needs.

```graphql
type Author {
    ...
    posts: [Post] @hasInverse(field: author)
    recentlyLiked: [Post]
}

type Post {
    ...
    author: Author
    numLikes: Int
}
```

Now, Dgraph will manage the connection between posts and authors and you can get on with concentrating on what your app needs to to - suggesting them interesting content.
