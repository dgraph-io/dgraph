+++
title = "Custom Queries"
weight = 3
[menu.main]
    parent = "custom"
+++

Let's say we want to integrate our app with an existing external REST API.  There's a few things we need to know:

* the URL of the API, the path and any parameters required
* the shape of the resulting JSON data
* the method (GET, POST, etc.), and
* what authorization we need to pass to the external endpoint

The custom query can take any number of scalar arguments and use those to construct the path, parameters and body (we'll see an example of that in the custom mutation section) of the request that gets sent to the remote endpoint.

In an app, you'd deploy an endpoint that does some custom work and returns data that's used in your UI, or you'd wrap some logic or call around an existing endpoint.  So that we can walk through a whole example, let's use the Twitter API.

To integrate a call that returns the data of Twitter user with our app, all we need to do is add the expected result type `TwitterUser` and set up a custom query:

```graphql
type TwitterUser @remote {
    id: ID!
    name: String
    screen_name: String
    location: String
    description: String
    followers_count: Int
    ...
}

type Query{
    getCustomTwitterUser(name: String!): TwitterUser @custom(http:{
        url: "https://api.twitter.com/1.1/users/show.json?screen_name=$name"
        method: "GET",
        forwardHeaders: ["Authorization"]
    })
}
```

Dgraph will then be able to accept a GraphQL query like

```graphql
query {
    getCustomTwitterUser(name: "dgraphlabs") {
        location
        description
        followers_count
    }
}
```

construct a HTTP GET request to `https://api.twitter.com/1.1/users/show.json?screen_name=dgraphlabs`, attach header `Authorization` from the incoming GraphQL request to the outgoing HTTP, and make the call and return a GraphQL result.

The result JSON of the actual HTTP call will contain the whole object from the REST endpoint (you can see how much is in the Twitter user object [here](https://developer.twitter.com/en/docs/tweets/data-dictionary/overview/user-object)).  But, the GraphQL query only asked for some of that, so Dgraph filters out any returned values that weren't asked for in the GraphQL query and builds a valid GraphQL response to the query and returns GraphQL.

```json
{
    "data": {
        "getCustomTwitterUser": { "location": ..., "description": ..., "followers_count": ... }
    }
}
```

Your version of the remote type doesn't have to be equal to the remote type.  For example, if you don't want to allow users to query the full Twitter user, you include in the type definition only the fields that can be queried.

All the usual options for custom queries are allowed; for example, you can have multiple queries in a single GraphQL request and a mix of custom and Dgraph generated queries, you can get the result compressed by setting `Accept-Encoding` to `gzip`, etc.

---
