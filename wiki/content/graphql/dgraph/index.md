+++
title = "GraphQL on Existing Dgraph"
weight = 13
[menu.main]
  url = "/graphql/dgraph/"
  identifier = "dgraph"
  parent = "graphql"
+++

<!-- <h1>{props.pageContext.frontmatter.title}</h1> -->

How to use GraphQL on an existing Dgraph instance.

If you have an existing Dgraph instance and want to also expose GraphQL, you need to add a GraphQL schema that maps to your Dgraph schema.  You don't need to expose your entire Dgraph schema as GraphQL, but do note that adding a GraphQL schema can alter the Dgraph schema.

Dgraph also allows type and edge names that aren't valid in GraphQL, so, often, you'll need to expose valid GraphQL names. Dgraph admits special characters and even different languages (see [here](https://docs.dgraph.io/query-language/#predicate-name-rules)), while the GraphQL Spec requires that type and field (predicate) names are generated from `/[_A-Za-z][_0-9A-Za-z]*/`.

# Mapping GraphQL to a Dgraph schema

By default, Dgraph generates a new predicate for each field in a GraphQL type. The name of the generated predicate is composed of the type name followed by a dot `.` and ending with the field name. Therefore, two different types with fields of the same name will turn out to be different Dgraph predicates and can have different indexes.  For example, the types:

```graphql
type Person {
    name: String @search(by: [hash])
    age: Int
}

type Movie {
    name: String @search(by: [term])
}
```

generate a Dgraph schema like:

```graphql
type Person {
    Person.name
    Person.age
}

type Movie {
    Movie.name
}

Person.name: string @index(hash) .
Person.age: int .
Movie.name: string @index(term) .
```

This behavior can be customized with the `@dgraph` directive.  

* `type T @dgraph(type: "DgraphType")` controls what Dgraph type is used for a GraphQL type.
* `field: SomeType @dgraph(pred: "DgraphPredicate")` controls what Dgraph predicate is mapped to a GraphQL field.

For example, if you have existing types that don't match GraphQL requirements, you can create a schema like the following.

```graphql
type Person @dgraph(type: "Human-Person") {
    name: String @search(by: [hash]) @dgraph(pred: "name")
    age: Int
}

type Movie @dgraph(type: "film") {
    name: String @search(by: [term]) @dgraph(pred: "film.name")
}
```

Which maps to the Dgraph schema:

```graphql
type Human-Person {
    name
    Person.age
}

type film {
    film.name
}

name string @index(hash) .
Person.age: int .
film.name string @index(term) .
```

You might also have the situation where you have used `name` for both movie names and people's names.  In this case you can map fields in two different GraphQL types to the one Dgraph predicate.

```graphql
type Person {
    name: String @dgraph(pred: "name")
    ...
}

type Movie {
    name: String @dgraph(pred: "name")
    ...
}
```

*Note: the current behavior requires that when two fields are mapped to the same Dgraph predicate both should have the same `@search` directive.  This is likely to change in a future release where the underlying Dgraph indexes will be the union of the `@search` directives, while the generated GraphQL API will expose only the search given for the particular field.  Allowing, for example, dgraph predicate name to have `term` and `hash` indexes, but exposing only term search for GraphQL movies and hash search for GraphQL people.*

# Roadmap

Be careful with mapping to an existing Dgraph instance.  Updating the GraphQL schema updates the underlying Dgraph schema. We understand that exposing a GraphQL API on an existing Dgraph instance is a delicate process and we plan on adding multiple checks to ensure the validity of schema changes to avoid issues caused by detectable mistakes.

Future features are likely to include:

* Generating a first pass GraphQL schema from an existing dgraph schema.
* A way to show what schema diff will happen when you apply a new GraphQL schema.
* Better handling of `@dgraph` with `@search`

We look forward to you letting us know what features you'd like, so please join us on [discuss](https://discuss.dgraph.io/) or [GitHub](https://github.com/dgraph-io/dgraph).
