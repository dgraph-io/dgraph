+++
title = "Introduction"
[menu.main]
    parent = "build-an-app-tutorial"
    identifier = "discuss-introduction"
    weight = 1   
+++

Welcome to Slash GraphQL and Dgraph.  Slash GraphQL is fully managed GraphQL platform and the fastest way to build GraphQL apps.

Building an app with Slash GraphQL means you can use GraphQL natively and don't have to build other tech stacks or work out how to translate from some other technology to GraphQL. You work only with GraphQL and, think in terms of the graph that matters for your app.

That's the process we'll walk through together in this tutorial, starting with ... , then, ...adfa

### What are wt going to build

Image of the app ![Rule](/images/graphql/tutorial/discuss/discuss-app-screenshot.png)

![Rule](/images/graphql/tutorial/discuss/discuss-app-post-screenshot.png)


### GraphQL, Dgraph and Graphs

You're familiar with GraphQL types, fields and resolvers. Maybe you've written an app that adds GraphQL over a REST endpoint or maybe over a relational database. So you know how GraphQL sits over those sources and issues many queries to translate the REST/relational data into something that looks like a graph.

You know there's a cognitive jump because your app is about a graph, but you've got to design a relational schema and work out how that translates as a graph; you'll think about the app in terms of the graph, but always have to mentally translate back and forth between the relational and graph models. There are engineering challenges around the translation as well as the efficiency of the queries.

There's none of that here.

Dgraph GraphQL is part of Dgraph, which stores a graph - it's a database of nodes and edges. So it's efficient to store, query and traverse as a graph. Your data will get stored just like you design it in the schema, and the queries are a single graph query that does just what the GraphQL query says.



With Dgraph you design your application in GraphQL. You design a set of GraphQL types that describes your requirements. Dgraph takes those types, prepares graph storage for them and generates a GraphQL API with queries and mutations.

You design a graph, store a graph and query a graph. You think and design in terms of the graph that your app is based around.

Let's move on to the design process - it's graph-first, in fact, it's GraphQL-first. We'll design the GraphQL types that our example app is based around, and ... we'll there's no and ... from that, you get a GraphQL API for those types; you just move on to building the app around it.







### Why GraphQL

### Why Slash GraphQL


This is a simple tutorial which will take you through making a basic todo app using Dgraph's GraphQL API and integrating it with Auth0.




### Steps




- [Schema Design](/graphql/todo-app-tutorial/todo-schema-design)
- [Basic UI](/graphql/todo-app-tutorial/todo-ui)
- [Add Auth Rules](/graphql/todo-app-tutorial/todo-auth-rules)
- [Use Auth0's JWT](/graphql/todo-app-tutorial/todo-auth0-jwt)
- [Deploy on Slash GraphQL](/graphql/todo-app-tutorial/deploy)

---
