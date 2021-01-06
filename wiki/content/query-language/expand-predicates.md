+++
date = "2017-03-20T22:25:17+11:00"
title = "Expand Predicates"
weight = 14
[menu.main]
    parent = "query-language"
+++

The `expand()` function can be used to expand the predicates out of a node. To
use `expand()`, the [type system]({{< relref "query-language/type-system.md" >}}) is required.
Refer to the section on the type system to check how to set the types
nodes. The rest of this section assumes familiarity with that section.

There are two ways to use the `expand` function.

* Types can be passed to `expand()` to expand all the predicates in the type.

Query example: List the movies from the Harry Potter series:

{{< runnable >}}
{
  all(func: eq(name@en, "Harry Potter")) @filter(type(Series)) {
    name@en
    expand(Series) {
      name@en
      expand(Film)
    }
  }
}
{{< /runnable >}}

* If `_all_` is passed as an argument to `expand()`, the predicates to be
expanded will be the union of fields in the types assigned to a given node.

The `_all_` keyword requires that the nodes have types. Dgraph will look for all
the types that have been assigned to a node, query the types to check which
attributes they have, and use those to compute the list of predicates to expand.

For example, consider a node that has types `Animal` and `Pet`, which have
the following definitions:

```
type Animal {
    name
    species
    dob
}

type Pet {
    owner
    veterinarian
}
```

When `expand(_all_)` is called on this node, Dgraph will first check which types
the node has (`Animal` and `Pet`). Then it will get the definitions of `Animal`
and `Pet` and build a list of predicates from their type definitions.

```
name
species
dob
owner
veterinarian
```

{{% notice "note" %}}
For `string` predicates, `expand` only returns values not tagged with a language
(see [language preference]({{< relref "query-language/graphql-fundamentals.md#language-support" >}})).  So it's often
required to add `name@fr` or `name@.` as well to an expand query.
{{% /notice  %}}

## Filtering during expand

Expand queries support filters on the type of the outgoing edge. For example,
`expand(_all_) @filter(type(Person))` will expand on all the predicates but will
only include edges whose destination node is of type Person. Since only nodes of
type `uid` can have a type, this query will filter out any scalar values.

Please note that other type of filters and directives are not currently supported
with the expand function. The filter needs to use the `type` function for the
filter to be allowed. Logical `AND` and `OR` operations are allowed. For
example, `expand(_all_) @filter(type(Person) OR type(Animal))` will only expand
the edges that point to nodes of either type.
