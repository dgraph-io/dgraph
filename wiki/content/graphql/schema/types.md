+++
title = "Types"
weight = 2
[menu.main]
    parent = "schema"
+++

This page describes how to use GraphQL types to set the a GraphQL schema for
Dgraph database.

### Scalars

Dgraph's GraphQL implementation comes with the standard GraphQL scalar types:
`Int`, `Float`, `String`, `Boolean` and `ID`.  There's also an `Int64` scalar,
and a `DateTime` scalar type that is represented as a string in RFC3339 format.

Scalar types, including `Int`, `Int64`, `Float`, `String` and `DateTime`; can be
used in lists. Lists behave like an unordered set in Dgraph. For example:
`["e1", "e1", "e2"]` may get stored as `["e2", "e1"]`, so duplicate values will
not be stored and order might not be preserved. All scalars may be nullable or
non-nullable.

{{% notice "note" %}}The `Int64` type introduced in release 20.11 represents
a signed integer ranging between `-(2^63)` and `(2^63 -1)`. Signed `Int64` values
in this range will be parsed correctly by Dgraph as long as the client can
serialize the number correctly in JSON. For example, a JavaScript client might
need to use a serialization library such as
[`json-bigint`](https://www.npmjs.com/package/json-bigint) to correctly
write an `Int64` value in JSON.{{% /notice %}}

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

### Union type

GraphQL Unions represent an object that could be one of a list of GraphQL Object types, but provides for no guaranteed fields between those types. So no fields may be queried on this type without the use of type refining fragments or inline fragments.

Union types have the potential to be invalid if incorrectly defined:

- A `Union` type must include one or more unique member types.
- The member types of a `Union` type must all be Object base types; [Scalar](#scalars), [Interface](#interfaces) and `Union` types must not be member types of a Union. Similarly, wrapping types must not be member types of a Union.


For example, the following defines the `HomeMember` union type:

```graphql
enum Category {
  Fish
  Amphibian
  Reptile
  Bird
  Mammal
  InVertebrate
}

interface Animal {
  id: ID!
  category: Category @search
}

type Dog implements Animal {
  breed: String @search
}

type Parrot implements Animal {
  repeatsWords: [String]
}

type Cheetah implements Animal {
  speed: Float
}

type Human {
  name: String!
  pets: [Animal!]!
}

union HomeMember = Dog | Parrot | Human

type Zoo {
  id: ID!
  animals: [Animal]
  city: String
}

type Home {
  id: ID!
  address: String
  members: [HomeMember]
}
```

So, when you want to query members in a `Home`, you will be able to do a GraphQL query like this:

```graphql
query {
  queryHome {
    address
    members {
      ... on Animal {
        category
      }
      ... on Dog {
        breed
      }
      ... on Parrot {
        repeatsWords
      }
      ... on Human {
        name
      }
    }
  }
}
```

And the results of the GraphQL query will look like the following:

```json
{
  "data": {
    "queryHome": {
      "address": "Earth",
      "members": [
        {
          "category": "Mammal",
          "breed": "German Shepherd"
        }, {
          "category": "Bird",
          "repeatsWords": ["Good Morning!", "I am a GraphQL parrot"]
        }, {
          "name": "Alice"
        }
      ]
    }
  }
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

### Geolocation types

Dgraph GraphQL comes with built-in types to store Geolocation data. Currently, it supports `Point`, `Polygon` and `MultiPolygon`. These types are useful in scenarios like storing a location's GPS coordinates, representing a city on the map, etc.

For example:

```graphql
type Hotel {
  id: ID!
  name: String!
  location: Point
  area: Polygon
}
```

#### Point

```graphql
type Point {
	longitude: Float!
	latitude: Float!
}
```

#### PointList

```graphql
type PointList {
	points: [Point!]!
}
```

#### Polygon

```graphql
type Polygon {
	coordinates: [PointList!]!
}
```

#### MultiPolygon

```graphql
type MultiPolygon {
	polygons: [Polygon!]!
}
```
