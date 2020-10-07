+++
title = "Custom Mutations"
weight = 4
[menu.main]
    parent = "custom"
+++

Let's say we have an application about authors and posts.  Logged in authors can add posts, but we want to do some input validation and add extra value when a post is added.  The key types might be as follows.

```graphql
type Author { ... }

type Post {
    id: ID:
    title: String
    text: String
    datePublished: DateTime
    author: Author
    ...
}
```

Dgraph generates an `addPost` mutation from those types, but we want to do something extra.  We don't want the `author` to come in with the mutation, that should get filled in from the JWT of the logged in user.  Also, the `datePublished` shouldn't be in the input; it should be set as the current time at point of mutation.  Maybe we also have some community guidelines about what might constitute an offensive `title` or `text` in a post. Maybe users can only post if they have enough community credit.

We'll need custom code to do all that, so we can write a custom function that takes in only the title and text of the new post.  Internally, it can check that the title and text satisfy the guidelines and that this user has enough credit to make a post. If those checks pass, it then builds a full post object by adding the current time as the `datePublished` and adding the `author` from the JWT information it gets from the forward header.  It can then call the `addPost` mutation constructed by Dgraph to add the post into Dgraph and returns the resulting post as its GraphQL output.

So as well as the types above, we need a custom mutation:

```graphql
type Mutation {
    newPost(title: String!, text: String): Post @custom(http:{
        url: "https://my.api.com/addPost"
        method: "POST",
        body: "{ postText: $text, postTitle: $title }"
        forwardHeaders: ["AuthHdr"]
    })
}
```

## Learn more

Find out more about how to turn off generated mutations and protecting mutations with authorization rules at:

* [Remote Types - Turning off Generated Mutations with `@remote` Directive](/graphql/custom/directive)
* [Securing Mutations with the `@auth` Directive](/graphql/authorization/mutations)

---
