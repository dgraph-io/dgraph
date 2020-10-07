+++
title = "Switching Backend Modes"
weight = 6
[menu.main]
    parent = "slash-graphql-admin"
+++

Slash GraphQL supports different 3 different backend modes, which controls how the underlying Dgraph instance is configured

### Readonly Mode

In readonly mode, only queries are allowed. All mutations and attempts to alter schema will be disallowed.

### GraphQL Mode

GraphQL mode is the default setting on Slash GraphQL, and is suitable for backends where the primary mode of interaction is via the GraphQL APIs. You can use of DQL/GraphQL+- queries and mutations, as described in the [advanced queries](/slash-graphql/advanced-queries/) section. However, all queries and mutations must be valid as per the applied GraphQL schema.

### Flexible Mode

Flexible mode is suitable for users who are already familiar with Dgraph, and intent to interact with their backend with DQL/GraphQL+-. Flexible mode removes any restrictions on queries and mutations, and also provides users access to advanced Dgraph features like directly altering the schema with the `/alter` http and GRPC endpoints.

Running your backend in flexible mode is also a requirement for upcoming features such as support for Dgraph's ACL.

## Changing your Backend Mode

You can change the backend mode on the [settings page](https://slash.dgraph.io/_/settings), under the "Advanced" tab.
