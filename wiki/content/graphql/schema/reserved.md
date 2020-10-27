+++
title = "Reserved Names"
weight = 1
[menu.main]
    parent = "schema"
+++

The following names are reserved and can't be used to define any other identifiers:

- `Int`
- `Float`
- `Boolean`
- `String`
- `DateTime`
- `ID`
- `uid`
- `Subscription`
- `as` (case-insensitive)
- `Query`
- `Mutation`
- `Point`
- `PointList`
- `Polygon`
- `MultiPolygon`


For each type, Dgraph generates a number of GraphQL types needed to operate the GraphQL API, these generated type names also can't be present in the input schema.  For example, for a type `Author`, Dgraph generates:

- `AuthorFilter`
- `AuthorOrderable`
- `AuthorOrder`
- `AuthorRef`
- `AddAuthorInput`
- `UpdateAuthorInput`
- `AuthorPatch`
- `AddAuthorPayload`
- `DeleteAuthorPayload`
- `UpdateAuthorPayload`

Thus if `Author` is present in the input schema, all of those become reserved type names.
