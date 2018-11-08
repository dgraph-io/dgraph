+++
title = "GraphQL+-: Tips and Tricks"
+++

## Get Sample Data

Use the `has` function to get some sample nodes.

{{< runnable >}}
{
  result(func: has(director.film), first: 10) {
    uid
    expand(_all_)
  }
}
{{< /runnable >}}


## Count number of connecting nodes

Use `expand(_all_)` to expand the nodes' edges, then assign them to a variable.
The variable can now be used to iterate over the unique neighboring nodes.
Then use `count(uid)` to count the number of nodes in a block.

{{< runnable >}}
{
  uids(func: has(director.film), first: 1) {
    uid
    expand(_all_) { u as uid }
  }

  result(func: uid(u)) {
    count(uid)
  }
}
{{< /runnable >}}

## Search on non-indexed predicates

Use the `has` function among the value variables to search on non-indexed predicates.

{{< runnable >}}
{
  var(func: has(pred_to_be_searched)) {
    p as pred_to_be_searched
  }
  query(func: eq(val(p), "value-searching-for")) {
    pred_to_be_searched
  }
}
{{< /runnable >}}