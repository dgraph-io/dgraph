+++
title = "Cached Results"
weight = 4
[menu.main]
    parent = "graphql-queries"
+++

Cached results can be used to serve read-heavy workloads with complex queries to improve performance. When cached results are enabled for a query, the stored results are served if queried within the defined time-to-live (TTL) of the cached query.

When using cached results, Dgraph will add the appropriate HTTP headers so the caching can be done at the browser or content delivery network (CDN) level.


{{% notice "note" %}}
Caching refers to external caching at the browser/CDN level. Internal caching at the database layer is not currently supported.
{{% /notice %}}

### Enabling cached results

To enable the external result cache you need to add the `@cacheControl(maxAge: int)` directive at the top of your query. This directive adds the appropriate `Cache-Control` HTTP headers to the response, so that browsers and CDNs can cache the results.

For example, the following query defines a cache with TTL of 15 seconds.

```graphql
query @cacheControl(maxAge: 15){
  queryReview(filter: { comment: {alloftext: "Fantastic"}}) {
    comment
    by {
      username
    }
    about {
      name
    }
  }
}
```

Dgraph's returned HTTP headers:

```
Cache-Control: public,max-age=15
Vary: Accept-Encoding
```
