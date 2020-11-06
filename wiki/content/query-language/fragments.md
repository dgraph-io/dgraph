+++
date = "2017-03-20T22:25:17+11:00"
title = "Fragments"
weight = 25
[menu.main]
    parent = "query-language"
+++

The `fragment` keyword lets you to define new fragments that can be referenced
in a query, per the [GraphQL specification](https://facebook.github.io/graphql/#sec-Language.Fragments).
Fragments allow for the reuse of common repeated selections of fields, reducing
duplicated text in the DQL documents. Fragments can be nested inside fragments,
but no cycles are allowed in such cases. For example:

```sh
curl -H "Content-Type: application/dql" localhost:8080/query -XPOST -d $'
query {
  debug(func: uid(1)) {
    name@en
    ...TestFrag
  }
}
fragment TestFrag {
  initial_release_date
  ...TestFragB
}
fragment TestFragB {
  country
}' | python -m json.tool | less
```

{{% notice "note" %}}
GraphQL+- has been renamed to Dgraph Query Language (DQL). While `application/dql`
is the preferred value for the `Content-Type` header, we will continue to support
`Content-Type: application/graphql+-` to avoid making breaking changes.
{{% /notice %}}
