+++
date = "2017-03-20T22:25:17+11:00"
title = "Query Language"
[menu.main]
  url = "/query-language/"
  identifier = "query-language"
  weight = 4
+++

Dgraph's GraphQL+- is based on Facebook's [GraphQL](https://facebook.github.io/graphql/).  GraphQL wasn't developed for Graph databases, but its graph-like query syntax, schema validation and subgraph shaped response make it a great language choice.  We've modified the language to better support graph operations, adding and removing features to get the best fit for graph databases.  We're calling this simplified, feature rich language, ''GraphQL+-''.

GraphQL+- is a work in progress. We're adding more features and we might further simplify existing ones.

## Take a Tour - https://dgraph.io/tour/

This document is the Dgraph query reference material.  It is not a tutorial.  It's designed as a reference for users who already know how to write queries in GraphQL+- but need to check syntax, or indices, or functions, etc.

{{% notice "note" %}}If you are new to Dgraph and want to learn how to use Dgraph and GraphQL+-, take the tour - https://dgraph.io/tour/{{% /notice %}}


### Running examples

The examples in this reference use a database of 21 million triples about movies and actors.  The example queries run and return results.  The queries are executed by an instance of Dgraph running at https://play.dgraph.io/.  To run the queries locally or experiment a bit more, see the [Getting Started]({{< relref "get-started/index.md" >}}) guide, which also shows how to load the datasets used in the examples here.

