+++
title = "And, Or and Not"
weight = 3
[menu.main]
    parent = "graphql-queries"
    name = "And, Or and Not"
+++

Every search filter contains `and`, `or` and `not`.

GraphQL's syntax is used to write these infix style, so "a and b" is written `a, and: { b }`, and "a or b or c" is written `a, or: { b, or: c }`.  Not is written prefix.

The posts that do not have "GraphQL" in the title.

```graphql
queryPost(filter: { not: { title: { allofterms: "GraphQL"} } } ) { ... }
```

The posts that have "GraphQL" or "Dgraph" in the title.

```graphql
queryPost(filter: {
    title: { allofterms: "GraphQL"},
    or: { title: { allofterms: "Dgraph" } }
  } ) { ... }
```

The posts that have "GraphQL" and "Dgraph" in the title.

```graphql
queryPost(filter: {
    title: { allofterms: "GraphQL"},
    and: { title: { allofterms: "Dgraph" } }
  } ) { ... }
```

The and is implicit for a single filter object, if the fields don't overlap.  For example, above the `and` is required because `title` is in both filters, where as below, `and` is not required.

```graphql
queryPost(filter: {
    title: { allofterms: "GraphQL" },
    datePublished: { ge: "2020-06-15" }
  } ) { ... }
```

The posts that have "GraphQL" in the title, or have the tag "GraphQL" and mention "Dgraph" in the title

```graphql
queryPost(filter: {
    title: { allofterms: "GraphQL"},
    or: { title: { allofterms: "Dgraph" }, tags: { eg: "GraphQL" } }
  } ) { ... }
```