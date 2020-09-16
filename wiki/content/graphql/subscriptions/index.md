+++
title = "GraphQL Subscriptions"
[menu.main]
  url = "/graphql/subscriptions/"
  name = "Subscriptions"
  identifier = "subscriptions"
  parent = "graphql"
  weight = 8
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

## Authorization with Subscriptions
Authorization adds more power to GraphQL subscriptions.You can use all the features of authorization which are there for queries.
In addition to them, You can also specify the timeout of the subscription in jwt after which the subscription automatically terminates.

## Example 

Consider following Schema, it has both @withSubscription and @auth directive defined on type Todo. Auth rule enforces that only todo's of owner $USER are visible which will be given in Jwt.

```graphql
type Todo @withSubscription @auth(
    	query: { rule: """
    		query ($USER: String!) {
    			queryTodo(filter: { owner: { eq: $USER } } ) {
    				__typename
    			}
   			}"""
     	}
   ){
        id: ID!
    	text: String! @search(by: [term])
     	owner: String! @search(by: [hash])
   }
# Dgraph.Authorization {"VerificationKey":"secret","Header":"Authorization","Namespace":"https://dgraph.io","Algo":"HS256"}
```
Subscription needs jwt in which $USER , expiry and other variables are declared. Below example shows generating Jwt and then using it with subscriptions.

![Subscription+Auth](/images/graphql/Auth+Subscription.gif "Subscription with Auth Example")

Generally jwt is passed in header from graphql client as key value pair, where key is Header given in schema and value is jwt.
For example in our case,key is Authorization and value is jwt. 

From apollo client this key-value pair is passed in connectionParams as follows.
```javascript
const wsLink = new WebSocketLink({
  uri: `wss://${ENDPOINT}`,
  options: {
    reconnect: true,
    connectionParams: {  "Authorization": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE2OTAxMjg2MjIsImh0dHBzOi8vZGdyYXBoLmlvIjp7IlJPTEUiOiJVU0VSIiwiVVNFUiI6IkFsaWNlIn0sImlzcyI6InRlc3QifQ.6AODlumsk9kbnwZHwy08l40PeqEmBHqK4E_ozNjQpuI", },});
```