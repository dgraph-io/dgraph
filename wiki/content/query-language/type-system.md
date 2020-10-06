+++
date = "2017-03-20T22:25:17+11:00"
title = "Type System"
weight = 21
[menu.main]
    parent = "query-language"
+++

Dgraph supports a type system that can be used to categorize nodes and query
them based on their type. The type system is also used during expand queries.

## Type definition

Types are defined using a GraphQL-like syntax. For example:

```
type Student {
  name
  dob
  home_address
  year
  friends
}
```

{{% notice "note" %}}You can't define type names starting with `dgraph.`, it is reserved as the
namespace for Dgraph's internal types/predicates. For example, defining `dgraph.Student` as a
type is invalid.{{% /notice  %}}

Types are declared along with the schema using the Alter endpoint. In order to
properly support the above type, a predicate for each of the attributes
in the type is also needed, such as:

```
name: string @index(term) .
dob: datetime .
home_address: string .
year: int .
friends: [uid] .
```

Reverse predicates can also be included inside a type definition. For example, the type above
could be expanded to include the parent of the student if there's a predicate `children` with
a reverse edge (the brackets around the predicate name are needed to properly understand the
special character `~`).

```
children: [uid] @reverse .

type Student {
  name
  dob
  home_address
  year
  friends
  <~children>
}
```

Edges can be used in multiple types: for example, `name` might be used for both
a person and a pet. Sometimes, however, it's required to use a different
predicate for each type to represent a similar concept. For example, if student
names and book names required different indexes, then the predicates must be
different.

```
type Student {
  student_name
}

type Textbook {
  textbook_name
}

student_name: string @index(exact) .
textbook_name: string @lang @index(fulltext) .
```

Altering the schema for a type that already exists, overwrites the existing
definition.

## Setting the type of a node

Scalar nodes cannot have types since they only have one attribute and its type
is the type of the node. UID nodes can have a type. The type is set by setting
the value of the `dgraph.type` predicate for that node. A node can have multiple
types. Here's an example of how to set the types of a node:

```
{
  set {
    _:a <name> "Garfield" .
    _:a <dgraph.type> "Pet" .
    _:a <dgraph.type> "Animal" .
  }
}
```

`dgraph.type` is a reserved predicate and cannot be removed or modified.

## Using types during queries

Types can be used as a top level function in the query language. For example:

```
{
  q(func: type(Animal)) {
    uid
    name
  }
}
```

This query will only return nodes whose type is set to `Animal`.

Types can also be used to filter results inside a query. For example:

```
{
  q(func: has(parent)) {
    uid
    parent @filter(type(Person)) {
      uid
      name
    }
  }
}
```

This query will return the nodes that have a parent predicate and only the
`parent`'s of type `Person`.

## Deleting a type

Type definitions can be deleted using the Alter endpoint. All that is needed is
to send an operation object with the field `DropOp` (or `drop_op` depending on
the client) to the enum value `TYPE` and the field 'DropValue' (or `drop_value`)
to the type that is meant to be deleted.

Below is an example deleting the type `Person` using the Go client:
```go
err := c.Alter(context.Background(), &api.Operation{
                DropOp: api.Operation_TYPE,
                DropValue: "Person"})
```

## Expand queries and types

Queries using [expand]({{< relref "query-language/expand-predicates.md" >}}) (i.e.:
`expand(_all_)`) require that the nodes to be expanded have types.
