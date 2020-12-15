+++
title = "Overview"
weight = 1
[menu.main]
    parent = "api"
    identifier = "api-overview"
+++

How to use the GraphQL API. 

Dgraph serves [spec-compliant
GraphQL](https://graphql.github.io/graphql-spec/June2018/) over HTTP to two endpoints: `/graphql` and `/admin`. 


In Slash GraphQL `/graphql` and `/admin` are served from the domain of your backend, which will be something like `https://YOUR-SUBDOMAIN.REGION.aws.cloud.dgraph.io`. If you are running a self-hosted Dgraph instance that will be at the alpha port and url (which defaults to `http://localhost:8080` if you aren't changing any settings).

In each case, both GET and POST requests are served.

- `/graphql` is where you'll find the GraphQL API for the types you've added. That is the single GraphQL entry point for your apps to Dgraph.

- `/admin` is where you'll find an admin API for administering your GraphQL instance. That's where you can update your GraphQL schema, perform health checks of your backend, and more.

This section covers the API served at `/graphql`. See [Admin](/graphql/admin) to learn more about the admin API.
