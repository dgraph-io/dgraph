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
  var(func: has(festival.date_founded)) {
    p as festival.date_founded
  }
  query(func: eq(val(p), "1961-01-01T00:00:00Z")) {
      uid
      name@en
      name@ru
      name@pl
      festival.date_founded
      festival.focus { name@en }
      festival.individual_festivals { total : count(uid) }
  }
}
{{< /runnable >}}
