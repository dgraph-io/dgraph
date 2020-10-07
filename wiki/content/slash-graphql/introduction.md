+++
title = "Introduction"
weight = 1   
[menu.main]
    parent = "slash-graphql"
+++

<p className="subheading font-weight-regular">Slash GraphQL Provides /graphql Backend for Your App</p>

Please see the following topics:

- The [QuickStart](/slash-graphql/slash-quick-start) will help you get started with a Slash GraphQL Schema, starting with a multi tenant todo app
- [Administering your Backend](/slash-graphql/admin/overview) covers topics such as how to programatically set your schema, and import or export your data
  - [Authentication](/slash-graphql/admin/authentication) will guide you in creating a API token. Since all admin APIs require an auth token, this is a good place to start.
  - [Schema](/slash-graphql/admin/schema) describes how to programatically query and update your GraphQL schema.
  - [Import and Exporting Data](/slash-graphql/admin/import-export) is a guide for exporting your data from a Slash GraphQL backend, and how to import it into another cluster
  - [Dropping Data](/slash-graphql/admin/drop-data) will guide you through dropping all data from your Slash GraphQL backend.
  - [Switching Backend Modes](/slash-graphql/admin/backend-modes) will guide you through changing Slash GraphQL backend mode.
- [Advanced Queries With GraphQL+-](/slash-graphql/advanced-queries) speaks about how to interact with your database via the gRPC endpoint.
- [One-click Deploy](/slash-graphql/one-click-deploy) speaks about how to deploy sample apps in a fresh instance of backend to start working with them.

You might also be interested in:

- [Dgraph GraphQL Schema Reference](/graphql/schema/schema-overview), which lists all the types and directives supported by Dgraph
- [Dgraph GraphQL API Reference](/graphql/api/api-overview), which serves as a guide to using your new `/graphql` endpoint
