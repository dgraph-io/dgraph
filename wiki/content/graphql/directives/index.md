+++
title = "Index of Directives"
weight = 11
[menu.main]
  url = "/graphql/directives/"
  name = "Directives"
  identifier = "directives"
  parent = "graphql"
+++

The list of all directives supported by Dgraph.

### @hasInverse

`@hasInverse` is used to setup up two way edges such that adding a edge in
one direction automically adds the one in the inverse direction.

Reference: [Linking nodes in the graph](/graphql/schema/graph-links)

### @search

`@search` allows you to perform filtering on a field while querying for nodes.

Reference: [Search](/graphql/schema/search)

### @dgraph

`@dgraph` directive tells us how to map fields within a type to existing predicates inside Dgraph.

Reference: [GraphQL on Existing Dgraph](/graphql/dgraph/)


### @id

`@id` directive is used to annotate a field which represents a unique identifier coming from outside
 of Dgraph.

Reference: [Identity](/graphql/schema/ids)

### @withSubscription

`@withSubscription` directive when applied on a type, generates subsciption queries for it.

Reference: [Subscriptions](/graphql/subscriptions)

### @secret

`@secret` directive is used to store secret information, it gets encrypted and then stored in Dgraph.

Reference: [Password Type](/graphql/schema/#password-type)

### @auth

`@auth` allows you to define how to apply authorization rules on the queries/mutation for a type.

Reference: [Auth directive](/graphql/authorization/directive)

### @custom

`@custom` directive is used to define custom queries, mutations and fields.

Reference: [Custom directive](/graphql/custom/directive)

### @remote

`@remote` directive is used to annotate types for which data is not stored in Dgraph. These types
are typically used with custom queries and mutations.

Reference: [Remote directive](/graphql/custom/directive)

### @cascade

`@cascade` allows you to filter out certain nodes within a query.

Reference: [Cascade](/graphql/queries/cascade)
