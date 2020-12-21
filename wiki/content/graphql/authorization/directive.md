+++
title = "The @auth directive"
weight = 2
[menu.main]
    parent = "authorization"
+++

Given an authentication mechanism and signed JWT, it's the `@auth` directive that tells Dgraph how to apply authorization.  The directive can be used on any type (that isn't a `@remote` type) and specifies the authorization for `query` as well as `add`, `update` and `delete` mutations.

In each case, `@auth` specifies rules that Dgraph applies during queries and mutations.  Those rules are expressed in exactly the same syntax as GraphQL queries.  Why?  Because the authorization you add to your app is about the graph of your application, so graph rules make sense.  It's also the syntax you already know about, you get syntax help from GraphQL tools in writing such rules, and it turns out to be exactly the kinds of rules Dgraph already knows how to evaluate.

Here's how the rules work.

## Authorization rules

A valid type and rule looks like the following.

```graphql
type Todo @auth(
    query: { rule: """
        query ($USER: String!) { 
            queryTodo(filter: { owner: { eq: $USER } } ) { 
                id 
            } 
        }"""
    }
){
    id: ID!
    text: String! @search(by: [term])
    owner: String! @search(by: [hash])
}
```

In addition to it, details of the authentication provider should be given in the last line of the schema, as discussed in section  [authorization-overview](/graphql/authorization/authorization-overview).

Here we define a type `Todo`, that's got an `id`, the `text` of the todo and the username of the `owner` of the todo.  What todos can a user query?  Any `Todo` that the `query` rule would also return.

The `query` rule in this case expects the JWT to contain a claim `"USER": "..."` giving the username of the logged in user, and says: you can query any todo that has your username as the owner.

In this example we use the `queryTodo` query that will be auto generated after uploading this schema. When using a query in a rule, you can only use the `queryTypeName` query. Where `TypeName` matches the name of the type, where the `@auth` directive is attached. In other words, we could not have used the `getTodo` query in our rule above to query by id only.

This rule is applied automatically at query time.  For example, the query

```graphql
query {
    queryTodo {
        id
        text
    }
}
```

will return only the todo's where `owner` equals `amit`, when Amit is logged in and only the todos owned by `nancy` when she's logged into your app.

Similarly,

```graphql
query {
    queryTodo(filter: { text: { anyofterms: "graphql"}}, first: 10, order: { asc: text }) {
        id
        text
    }
}
```

will return the first ten todos, ordered ascending by title of the user that made the query.

This means your frontend doesn't need to be sensitive to the auth rules.  Your app can simply query for the todos and that query behaves properly depending on who's logged in.

In general, an auth rule should select a field that's expected to exist at the inner most field, often that's the `ID` or `@id` field.  Auth rules are run in a mode that requires all fields in the rule to find a value in order to succeed.  

## Graph traversal in auth rules

Often authorization depends not on the object being queried, but on the connections in the graph that object has or doesn't have.  Because the auth rules are graph queries, they can express very powerful graph search and traversal.

For a simple todo app, it's more likely that you'll have types like this:

```graphql
type User {
    username: String! @id
    todos: [Todo]
}

type Todo {
    id: ID!
    text: String!
    owner: User
}
```

This means your auth rule for todos will depend not on a value in the todo, but on checking which owner it's linked to.  This means our auth rule must make a step further into the graph to check who the owner is.

```graphql
query ($USER: String!) { 
    queryTodo {
        owner(filter: { username: { eq: $USER } } ) { 
            username
        } 
    } 
}
```

You can express a lot with these kinds of graph traversals.  For example, multitenancy rules can express that you can only see an object if it's linked (through what ever graph search you define) to the organization you were authenticated from.  That means your app can split data per customer easily.

You can also express rules that can be administered by the app itself.  You might define type `Role` and enum `Privileges` that can have values like `VIEW`, `ADD`, etc. and state in your auth rules that a user needs to have a role with particular privileges to query/add/update/delete and those roles can then be allocated inside the app.  For example, in an app about project management, when a project is created the admin can decide which users have view or edit permission, etc.

## Role Based Access Control

As well as rules that relate a user's claims to a graph traversal, role based access control rules are also possible.  These rules relate a claim in the JWT to a known value.  

For example, perhaps only someone logged in with the `ADMIN` role is allowed to delete users.  For that, we might expect the JWT to contain a claim `"ROLE": "ADMIN"`, and can thus express a rule that only allows users with the `ADMIN` claim to delete.

```graphql
type User @auth(
    delete: { rule:  "{$ROLE: { eq: \"ADMIN\" } }"}
) { 
    username: String! @id
    todos: [Todo]
}
```

Not all claims need to be present in all JWTs.  For example, if the `ROLE` claim isn't present in a JWT, any rule that relies on `ROLE` simply evaluates to false.  As well as simplifying your JWTs (e.g. not all users need a role if it doesn't make sense to do so), this means you can also simply disallow some queries and mutations.  If you know that your JWTs never contain the claim `DENIED`, then a rule such as

```graphql
type User @auth(
    delete: { rule:  "{$DENIED: { eq: \"DENIED\" } }"}
) { 
    ...
}
```

can never be true and this would prevent users ever being deleted.

## and, or & not 

Rules can be combined with the logical connectives and, or and not, so a permission can be a mixture of graph traversals and role based rules.

In the todo app, you can express, for example, that you can delete a `Todo` if you are the author, or are the site admin.

```graphql
type Todo @auth(
    delete: { or: [ 
        { rule:  "query ($USER: String!) { ... }" }, # you are the author graph query
        { rule:  "{$ROLE: { eq: \"ADMIN\" } }" }
    ]}
)
```

## Public Data

Many apps have data that can be accessed by anyone, logged in or not.  That also works nicely with Dgraph auth rules.  

For example, in Twitter, StackOverflow, etc. you can see authors and posts without being signed it - but you'd need to be signed in to add a post.  With Dgraph auth rules, if a type doesn't have, for example, a `query` auth rule or the auth rule doesn't depend on a JWT value, then the data can be accessed without a signed JWT.

For example, the todo app might allow anyone, logged in or not, to view any author, but not make any mutations unless logged in as the author or an admin.  That would be achieved by rules like the following.

```graphql
type User @auth(
    # no query rule
    add: { rule:  "{$ROLE: { eq: \"ADMIN\" } }" },
    update: ...
    delete: ...
) {
    username: String! @id
    todos: [Todo]
}
```

Maybe some todos can be marked as public and users you aren't logged in can see those.

```graphql
type Todo @auth(
    query: { or: [
        # you are the author 
        { rule: ... },
        # or, the todo is marked as public
        { rule: """query { 
            queryTodo(filter: { isPublic: { eq: true } } ) { 
                id 
            } 
        }"""}
    ]}
) { 
    ...
    isPublic: Boolean
}

```

Because the rule doesn't depend on a JWT value, it can be successfully evaluated for users who aren't logged in.

Ensuring that requests are from an authenticated JWT, and no further restrictions, can be done by arranging the JWT to contain a value like `"isAuthenticated": "true"`.  For example,


```graphql
type User @auth(
    query: { rule:  "{$isAuthenticated: { eq: \"true\" } }" },
) {
    username: String! @id
    todos: [Todo]
}
```

specifies that only authenticated users can query other users.

---
