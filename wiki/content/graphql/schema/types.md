+++
title = "Types"
weight = 2
[menu.main]
    parent = "schema"
+++

This page describes how you use GraphQL types to set the Dgraph GraphQL schema.

### Scalars

Dgraph GraphQL comes with the standard GraphQL scalars: `Int`, `Float`, `String`, `Boolean` and `ID`.  There's also a `DateTime` scalar - represented as a string in RFC3339 format.

Scalars `Int`, `Float`, `String` and `DateTime` can be used in lists. Note that lists behave like an unordered set in Dgraph. For example: `["e1", "e1", "e2"]` may get stored as `["e2", "e1"]`, i.e., duplicate values will not be stored and order may not be preserved.

All scalars may be nullable or non-nullable.

The `ID` type is special.  IDs are auto-generated, immutable, and can be treated as strings.  Fields of type `ID` can be listed as nullable in a schema, but Dgraph will never return null.

* *Schema rule*: `ID` lists aren't allowed - e.g. `tags: [String]` is valid, but `ids: [ID]` is not.
* *Schema rule*: Each type you define can have at most one field with type `ID`.  That includes IDs implemented through interfaces.

It's not possible to define further scalars - you'll receive an error if the input schema contains the definition of a new scalar.

For example, the following GraphQL type uses all of the available scalars.

```graphql
type User {
    userID: ID!
    name: String!
    lastSignIn: DateTime
    recentScores: [Float]
    reputation: Int
    active: Boolean
}
```

Scalar lists in Dgraph act more like sets, so `tags: [String]` would always contain unique tags.  Similarly, `recentScores: [Float]` could never contain duplicate scores.

### Enums

You can define enums in your input schema.  For example:

```graphql
enum Tag {
    GraphQL
    Database
    Question
    ...
}

type Post {
    ...
    tags: [Tag!]!
}
```

### Types

From the built-in scalars and the enums you add, you can generate types in the usual way for GraphQL.  For example:

```graphql
enum Tag {
    GraphQL
    Database
    Dgraph
}

type Post {
    id: ID!
    title: String!
    text: String
    datePublished: DateTime
    tags: [Tag!]!
    author: Author!
}

type Author {
    id: ID!
    name: String!
    posts: [Post!]
    friends: [Author]
}
```

* *Schema rule*: Lists of lists aren't accepted.  For example: `multiTags: [[Tag!]]` isn't valid.
* *Schema rule*: Fields with arguments are not accepted in the input schema unless the field is implemented using the `@custom` directive.

### Interfaces

GraphQL interfaces allow you to define a generic pattern that multiple types follow.  When a type implements an interface, that means it has all fields of the interface and some extras.  

When a type implements an interface, GraphQL requires that the type repeats all the fields from the interface, but that's just boilerplate and a maintenance problem, so Dgraph doesn't need that repetition in the input schema and will generate the correct GraphQL for you.

For example, the following defines the schema for posts with comment threads; Dgraph will fill in the `Question` and `Comment` types to make the full GraphQL types.

```graphql
interface Post {
    id: ID!
    text: String
    datePublished: DateTime
}

type Question implements Post {
    title: String!
}

type Comment implements Post {
    commentsOn: Post!
}
```

The generated GraphQL will contain the full types, for example, `Question` gets expanded as:

```graphql
type Question implements Post {
    id: ID!
    text: String
    datePublished: DateTime
    title: String!
}
```

while `Comment` gets expanded as:

```graphql
type Comment implements Post {
    id: ID!
    text: String
    datePublished: DateTime
    commentsOn: Post!
}
```

### Password type
A password for an entity is set with setting the schema for the node type with `@secret` directive. Passwords cannot be queried directly, only checked for a match using the `checkTypePassword` function where `Type` is the node type.
The passwords are encrypted using [bcrypt](https://en.wikipedia.org/wiki/Bcrypt).

For example, to set a password, first set schema:

1. Cut-and-paste the following schema into a file called `schema.graphql`
    ```graphql
    type Author @secret(field: "pwd") {
      name: String! @id
    }
    ```

2. Run the following curl request:
    ```bash
    curl -X POST localhost:8080/admin/schema --data-binary '@schema.graphql'
    ```

3. Set the password by pointing to the `graphql` endpoint (http://localhost:8080/graphql):
    ```graphql
    mutation {
      addAuthor(input: [{name:"myname", pwd:"mypassword"}]) {
        author {
          name
        }
      }
    }
    ```

The output should look like:
```json
{
  "data": {
    "addAuthor": {
      "author": [
        {
          "name": "myname"
        }
      ]
    }
  }
}
```

You can check a password:
```graphql
query {
  checkAuthorPassword(name: "myname", pwd: "mypassword") {
   name
  }
}
```

output:
```json
{
  "data": {
    "checkAuthorPassword": {
      "name": "myname"
    }
  }
}
```

If the password is wrong you will get the following response:
```json
{
  "data": {
    "checkAuthorPassword": null
  }
}
```
