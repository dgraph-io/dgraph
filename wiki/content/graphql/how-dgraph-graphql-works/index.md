+++
title = "How GraphQL works within Dgraph"
weight = 2
[menu.main]
  url = "/graphql/how-dgraph-graphql-works/"
  name = "How GraphQL works within Dgraph"
  identifier = "how-dgraph-graphql-works"
  parent = "graphql"
+++

Dgraph is a GraphQL database.  That means, with Dgraph, you design your application in GraphQL, you iterate on your app in GraphQL and, when you need it, you scale with GraphQL.  

You design a set of GraphQL types that describes your requirements.  Dgraph takes those types, prepares graph storage for them and generates a GraphQL API with queries and mutations.

You design a graph, store a graph and query a graph.  You think and design in terms of the graph that your app is based around.

Let's look at how that might work a simple Twitter clone.

## The app building process

You'll have an idea for your app, maybe you've sketched out the basic UI, or maybe you've worked out the basic things in your app and their relationships.  From that, you can derive a first version of your schema.

```graphql
type User {
    username: String! @id
    tweets: [Tweet]
}

type Tweet {
    text: String!
}
```

Load that into Dgraph, and you'll have a working GraphQL API.  You can start doing example queries and mutations with a tool like GraphQL Playground or Insomnia, you can even jump straight in and start building a UI with, say, Apollo Client.  That's how quickly you can get started.

Soon, you'll need to iterate while you are developing, or need to produce the next version of your idea.  Either way, Dgraph makes it easy to iterate on your app.  Add extra fields, add search, and Dgraph adjusts.

```graphql
type User {
    username: String! @id
    tweets: [Tweet]
}

type Tweet {
    text: String! @search(by: [fulltext])
    datePosted: DateTime
}
```

You can even do data migrations in GraphQL, so you never have to think about anything other than GraphQL.

Eventually, you'll need custom business logic and bespoke code to enhance your GraphQL server.  You can write that code however works best for your app and then integrate it directly into your GraphQL schema.

```graphql
type User {
    ...
}

type Tweet {
    ...
    myCustomField @custom(...)
}

type Query {
    MyCustomQuery @custom(...)
}
```

Again, Dgraph adjusts, and you keep working on your app, not on translating another data format into a graph.

## GraphQL, Dgraph and Graphs

You might be familiar with GraphQL types, fields and resolvers.  Perhaps you've written an app that adds GraphQL over a REST endpoint or maybe over a relational database.  If so, you know how GraphQL sits over those sources and issues many queries to translate the REST/relational data into something that looks like a graph.  

There's a cognitive jump in that process because your app is about a graph, but you've got to design a relational schema and work out how that translates as a graph.  You'll be thinking about the app in terms of the graph, but have to mentally translate back and forth between the relational and graph models.  There are engineering challenges around the translation as well as the efficiency of the queries.  

There's none of that with Dgraph.  

Dgraph GraphQL is part of Dgraph, which stores a graph - it's a database of nodes and edges.  So it's efficient to store, query and traverse as a graph.  Your data will get stored just like you design it in the schema, and the queries are a single graph query that does just what the GraphQL query says.
