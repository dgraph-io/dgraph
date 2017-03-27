+++
title = "GraphQL+-"
weight = 10

[menu.main]
    url = "graphql+-"
    parent = "query-language"

+++

Dgraph uses a variation of [GraphQL](https://facebook.github.io/graphql/) as the primary language of communication.
GraphQL is a query language created by Facebook for describing the capabilities and requirements of data models for client‐server applications.
While GraphQL isn't aimed at Graph databases, it's graph-like query syntax, schema validation and subgraph shaped response make it a great language choice.
Having said that, we have modified GraphQL to support graph operations and removed some of the features that we felt weren't a right fit to be a language for a graph database.
We're calling this simplified, feature rich language, ''GraphQL+-''.

{{Note|This language is a work in progress. We're adding more features and we might further simplify some of the existing ones.}}
