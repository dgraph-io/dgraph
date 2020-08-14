+++
title = "Cascade"
[menu.main]
    parent = "graphql-queries"
    name = "Cascade"
    weight = 5   
+++

`@cascade` is available as a directive which can be applied on fields. With the @cascade
directive, nodes that don’t have all fields specified in the query are removed.
This can be useful in cases where some filter was applied and some nodes might not
have all listed fields.

For example, the query below would only return the authors which have both reputation
and posts and where posts have text. Note that `@cascade` trickles down so it would
automatically be applied at the `posts` level as well if its applied at the `queryAuthor`
level.

```graphql
{
    queryAuthor @cascade {
        reputation
        posts {
            text
        }
    }
}
```

`@cascade` can also be used at nested levels, so the query below would return all authors
but only those posts which have both `text` and `id`.

```graphql
{
    queryAuthor  {
        reputation
        posts @cascade {
            id
            text
        }
    }
}
```