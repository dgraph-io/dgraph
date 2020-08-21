+++
date = "2017-03-20T22:25:17+11:00"
title = "Delete"
[menu.main]
    parent = "mutations"
    weight = 7
+++

A delete mutation, signified with the `delete` keyword, removes triples from the store.

For example, if the store contained
```RDF
<0xf11168064b01135b> <name> "Lewis Carrol"
<0xf11168064b01135b> <died> "1998"
<0xf11168064b01135b> <dgraph.type> "Person" .
```

Then delete mutation

```sh
{
  delete {
     <0xf11168064b01135b> <died> "1998" .
  }
}
```

Deletes the erroneous data and removes it from indexes if present.

For a particular node `N`, all data for predicate `P` (and corresponding indexing) is removed with the pattern `S P *`.

```sh
{
  delete {
     <0xf11168064b01135b> <author.of> * .
  }
}
```

The pattern `S * *` deletes all the known edges out of a node, any reverse edges corresponding to the removed
edges and any indexing for the removed data. The predicates to delete are
derived from the type information for that node (the value of the `dgraph.type`
edges on that node and their corresponding definitions in the schema). If the type information
for the node `S` is missing then the delete operation will not work.


```sh
{
  delete {
     <0xf11168064b01135b> * * .
  }
}
```

If the node `S` in the delete pattern `S * *` has only a few predicates as a part of `dgraph.type`,
then only those predicates will be deleted which are part of `dgraph.type`. It implies that after a delete operation
by this pattern, the node itself may still exist depending on if all the predicates are part of `dgraph.type` or not.

{{% notice "note" %}} The patterns `* P O` and `* * O` are not supported since its expensive to store/find all the incoming edges. {{% /notice %}}

## Deletion of non-list predicates

Deleting the value of a non-list predicate (i.e a 1-to-1 relation) can be done in two ways.

1. Using the star notation mentioned in the last section.
1. Setting the object to a specific value. If the value passed is not the current value, the mutation will succeed but will have no effect. If the value passed is the current value, the mutation will succeed and will delete the triple.

For language-tagged values, the following special syntax is supported:

```
{
  delete {
    <0x12345> <name@es> * .
  }
}
```

In this example, the value of name tagged with language tag `es` will be deleted.
Other tagged values are left untouched.