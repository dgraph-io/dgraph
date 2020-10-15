+++
title = "GraphQL Error Propagation"
weight = 6
[menu.main]
    parent = "api"
    name = "GraphQL Errors"
+++

<!-- this needs something else?  probably an example to help explain better?-->

Before returning query and mutation results, Dgraph uses the types in the schema to apply GraphQL [value completion](https://graphql.github.io/graphql-spec/June2018/#sec-Value-Completion) and [error handling](https://graphql.github.io/graphql-spec/June2018/#sec-Errors-and-Non-Nullability).  That is, `null` values for non-nullable fields, e.g. `String!`, cause error propagation to parent fields.  

In short, the GraphQL value completion and error propagation mean the following.

* Fields marked as nullable (i.e. without `!`) can return `null` in the json response.
* For fields marked as non-nullable (i.e. with `!`) Dgraph never returns null for that field.
* If an instance of type has a non-nullable field that has evaluated to null, the whole instance results in null.
* Reducing an object to null might cause further error propagation.  For example, querying for a post that has an author with a null name results in null: the null name (`name: String!`) causes the author to result in null, and a null author causes the post (`author: Author!`) to result in null.
* Error propagation for lists with nullable elements, e.g. `friends [Author]`, can result in nulls inside the result list.
* Error propagation for lists with non-nullable elements results in null for `friends [Author!]` and would cause further error propagation for `friends [Author!]!`.

Note that, a query that results in no values for a list will always return the empty list `[]`, not `null`, regardless of the nullability.  For example, given a schema for an author with `posts: [Post!]!`, if an author has not posted anything and we queried for that author, the result for the posts field would be `posts: []`.  

A list can, however, result in null due to GraphQL error propagation.  For example, if the definition is `posts: [Post!]`, and we queried for an author who has a list of posts.  If one of those posts happened to have a null title (title is non-nullable `title: String!`), then that post would evaluate to null, the `posts` list can't contain nulls and so the list reduces to null.
