+++
date = "2017-03-20T22:25:17+11:00"
title = "Overview"
weight = 1   
[menu.main]
    parent = "slash-graphql-admin"
    name = "Overview"
    identifier = "slash-overview"
+++

*These are draft docs for Slash GraphQL, which is currently in beta*

Here is a guide to programatically administering your Slash GraphQL backend.

Wherever possible, we have maintained compatibility with the corresponding Dgraph API, with the additional step of requiring authentication via the 'X-Auth-Token' header.

Please see the following topics:

* [Authentication](/slash-graphql/admin/authentication) will guide you in creating a API token. Since all admin APIs require an auth token, this is a good place to start.
* [Schema](/slash-graphql/admin/schema) describes how to programatically query and update your GraphQL schema.
* [Import and Exporting Data](/slash-graphql/admin/import-export) is a guide for exporting your data from a Slash GraphQL backend, and how to import it into another cluster
* [Dropping Data](/slash-graphql/admin/drop-data) will guide you through dropping all data from your Slash GraphQL backend.
