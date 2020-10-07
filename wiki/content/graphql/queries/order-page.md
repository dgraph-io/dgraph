+++
title = "Order and Pagination"
weight = 4
[menu.main]
    parent = "graphql-queries"
    name = "Order and Pagination"
+++

Every type with fields whose types can be ordered (`Int`, `Float`, `String`, `DateTime`) gets
ordering built into the query and any list fields of that type. Every query and list field
gets pagination with `first` and `offset` and ordering with `order` parameter.

For example, find the most recent 5 posts.

```graphql
queryPost(order: { desc: datePublished }, first: 5) { ... }
```

Skip the first five recent posts and then get the next 10.

```graphql
queryPost(order: { desc: datePublished }, offset: 5, first: 10) { ... }
```

It's also possible to give multiple orders.  For example, sort by date and within each
date order the posts by number of likes.

```graphql
queryPost(order: { desc: datePublished, then: { desc: numLikes } }, first: 5) { ... }
```