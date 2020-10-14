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

## Authorization with Subscriptions

Authorization adds more power to GraphQL subscriptions. You can use all the features of authorization that are there for queries.
In addition to them, You can also specify the timeout of the subscription in the JWT after which the subscription automatically terminates.

## Example 

### schema
Consider following Schema, it has both `@withSubscription` and `@auth` directive defined on type Todo. Auth rule enforces that only todo's of owner `$USER` is visible which will be given in the JWT.

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

### JWT

Subscription needs the JWT in which `$USER`, expiry, and other variables are declared. 
The JWT is passed from GraphQL client as key-value pair, where the key is Header given in schema and the value is the JWT.
For example in our case, the key is Authorization and the value is the JWT. 

Most of the GraphQL clients have a separate header section to pass Header-JWT key-value pair while from apollo client it is passed
in connectionParams as follows.
```javascript
const wsLink = new WebSocketLink({
  uri: `wss://${ENDPOINT}`,
  options: {
    reconnect: true,
    connectionParams: {  "Authorization": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE2OTAxMjg2MjIsImh0dHBzOi8vZGdyYXBoLmlvIjp7IlJPTEUiOiJVU0VSIiwiVVNFUiI6IkFsaWNlIn0sImlzcyI6InRlc3QifQ.6AODlumsk9kbnwZHwy08l40PeqEmBHqK4E_ozNjQpuI", },});
```

### Working

The below example shows the working of Subscription with Auth rules for the schema given above.

First, we generate the JWT as shown in the below image with expiry and `$USER` which is the owner of TODO.
You can generate the JWT from [jwt.io](https://jwt.io/)
We need to send the JWT to the server along with the request as discussed above.

![Subscription-Generating-JWT](/images/graphql/Generating-JWT.png "Subscription with Auth Example")


Next, We run the subscription and send updates. We see that only the Todo's which are added with the owner name Alice are visible in the subscription.

![Subscription+Auth-Action](/images/graphql/Auth-Action.gif "Subscription with Auth Example")


And after some time the JWT expires and the subscription terminates as shown below.

![Subscription+Timeout](/images/graphql/Subscription-Timeout.gif "Subscription with Auth Example")

