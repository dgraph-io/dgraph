+++
title = "GraphQL Subscriptions"
weight = 8
[menu.main]
  url = "/graphql/subscriptions/"
  name = "Subscriptions"
  identifier = "subscriptions"
  parent = "graphql"
+++

Subscriptions allow clients to listen to real-time messages from the server. The client connects to the server via a bi-directional communication channel using WebSocket and sends a subscription query that specifies which event it is interested in. When an event is triggered, the server executes the stored GraphQL query, and the result is sent through the same communication channel back to the client.

The client can unsubscribe by sending a message to the server. The server can also unsubscribe at any time due to errors or timeout. Another significant difference between queries/mutations and a subscription is that subscriptions are stateful and require maintaining the GraphQL document, variables, and context over the lifetime of the subscription.

![Subscription](/images/graphql/subscription_flow.png "Subscription in GraphQL")

## Enable Subscriptions

In GraphQL, it's straightforward to enable subscriptions on any type. We add the directive `@withSubscription` in the schema along with the type definition.

```graphql
type Todo @withSubscription {
  id: ID!
  title: String!
  description: String!
  completed: Boolean!
}
```

## Example

Once the schema is added, you can fire a subscription query, and we receive updates when the subscription query result is updated.

![Subscription](/images/graphql/subscription_example.gif "Subscription Example")

## Apollo Client Setup

Here is an excellent blog explaining in detail on [how to set up GraphQL Subscriptions using Apollo client](https://dgraph.io/blog/post/how-does-graphql-subscription/).
