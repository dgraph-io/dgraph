+++
title = "Overview"
weight = 1
[menu.main]
    parent = "schema"
    identifier = "schema-overview"
+++

This section describes all the things you can put in your input GraphQL schema, and what gets generated from that.

The process for serving GraphQL with Dgraph is to add a set of GraphQL type definitions using the `/admin` endpoint.  Dgraph takes those definitions, generates queries and mutations, and serves the generated GraphQL schema.  

The input schema may contain interfaces, types and enums that follow the usual GraphQL syntax and validation rules. 

If you want to make your schema editing experience nicer, you should use an editor that does syntax highlighting for GraphQL.  With that, you may also want to include the definitions [here](/graphql/schema/dgraph-schema) as an import.
