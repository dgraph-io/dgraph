+++
date = "2017-03-20T22:25:17+11:00"
title = "Recurse Query"
weight = 24
[menu.main]
    parent = "query-language"
+++

`Recurse` queries let you traverse a set of predicates (with filter, facets, etc.) until we reach all leaf nodes or we reach the maximum depth which is specified by the `depth` parameter.

To get 10 movies from a genre that has more than 30000 films and then get two actors for those movies we'd do something as follows:
{{< runnable >}}
{
	me(func: gt(count(~genre), 30000), first: 1) @recurse(depth: 5, loop: true) {
		name@en
		~genre (first:10) @filter(gt(count(starring), 2))
		starring (first: 2)
		performance.actor
	}
}
{{< /runnable >}}
Some points to keep in mind while using recurse queries are:

- You can specify only one level of predicates after root. These would be traversed recursively. Both scalar and entity-nodes are treated similarly.
- Only one recurse block is advised per query.
- Be careful as the result size could explode quickly and an error would be returned if the result set gets too large. In such cases use more filters, limit results using pagination, or provide a depth parameter at root as shown in the example above.
- The `loop` parameter can be set to false, in which case paths which lead to a loop would be ignored
  while traversing.
- If not specified, the value of the `loop` parameter defaults to false.
- If the value of the `loop` parameter is false and depth is not specified, `depth` will default to `math.MaxUint64`, which means that the entire graph might be traversed until all the leaf nodes are reached.

