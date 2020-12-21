+++
title = "The @custom directive"
weight = 2
[menu.main]
    parent = "custom"
+++

The `@custom` directive is used to define custom queries, mutations and fields.

In all cases, the result type (of the query, mutation or field) can be either:

* a type that's stored in Dgraph (that's any type you've defined in your schema), or
* a type that's not stored in Dgraph and is marked with the `@remote` directive.

Because the result types can be local or remote, you can call other HTTP endpoints, call remote GraphQL, or even call back to your Dgraph instance to add extra logic on top of Dgraph's graph search or mutations.

Here's the GraphQL definition of the directives:

```graphql
directive @custom(http: CustomHTTP) on FIELD_DEFINITION
directive @remote on OBJECT | INTERFACE

input CustomHTTP {
	url: String!
	method: HTTPMethod!
	body: String
	graphql: String
	mode: Mode
	forwardHeaders: [String!]
	secretHeaders: [String!]
	introspectionHeaders: [String!]
	skipIntrospection: Boolean
}

enum HTTPMethod { GET POST PUT PATCH DELETE }
enum Mode { SINGLE BATCH }
```

Each definition of custom logic must include:

* the `url` where the custom logic is called.  This can include a path and parameters that depend on query/mutation arguments or other fields.
* the HTTP `method` to use in the call.  For example, when calling a REST endpoint with `GET`, `POST`, etc.

Optionally, the custom logic definition can also include:

* a `body` definition that can be used to construct a HTTP body from from arguments or fields.
* a list of `forwardHeaders` to take from the incoming request and add to the outgoing HTTP call.
Used, for example, if the incoming request contains an auth token that must be passed to the custom logic.
* a list of `secretHeaders` to take from the `Dgraph.Secret` defined in the schema file and add to the outgoing HTTP call.
Used, for example, for a server side API key and other static value that must be passed to the custom logic.
* the `graphql` query/mutation to call if the custom logic is a GraphQL server and whether to introspect or not (`skipIntrospection`) the remote GraphQL endpoint.
* `mode` which is used for resolving fields by calling an external GraphQL query/mutation. It can either be `BATCH` or `SINGLE`.
* a list of `introspectionHeaders` to take from the `Dgraph.Secret` defined in the schema file and added to the
introspection requests sent to the `graphql` query/mutation.


The result type of custom queries and mutations can be any object type in your schema, including `@remote` types.  For custom fields the type can be object types or scalar types.

The `method` can be any of the HTTP methods: `GET`, `POST`, `PUT`, `PATCH`, or `DELETE`, and `forwardHeaders` is a list of headers that should be passed from the incoming request to the outgoing HTTP custom request.  Let's look at each of the other `http` arguments in detail.

## Dgraph.Secret

Sometimes you might want to forward some static headers to your custom API which can't be exposed
to the client. This could be an API key from a payment processor or an auth token for your organization
on GitHub. These secrets can be specified as comments in the schema file and then can be used in
`secretHeaders` and `introspectionHeaders` while defining the custom directive for a field/query.


```graphql
	type Query {
		getTopUsers(id: ID!): [User] @custom(http: {
			url: "http://api.github.com/topUsers",
			method: "POST",
			introspectionHeaders: ["Github-Api-Token"],
			secretHeaders: ["Authorization:Github-Api-Token"],
            graphql: "..."
		})
}

# Dgraph.Secret Github-Api-Token "long-token"
```

In the above request, `Github-Api-Token` would be sent as a header with value `long-token` for
the introspection request. For the actual request, the value `Authorization` would be sent along with
the value `long-token`. Note `Authorization:Github-Api-Token` syntax tells us to use the value for the
`Github-Api-Token` dgraph secret but to forward it to the custom API with the header key as `Authorization`.


## The URL and method

The URL can be as simple as a fixed URL string, or include details drawn from the arguments or fields.

A simple string might look like:

```graphql
type Query {
    myCustomQuery: MyResult @custom(http: {
        url: "https://my.api.com/theQuery",
        method: GET
    })
}
```

While, in more complex cases, the arguments of the query/mutation can be used as a pattern for the URL:

```graphql
type Query {
    myGetPerson(id: ID!): Person @custom(http: {
        url: "https://my.api.com/person/$id",
        method: GET
    })

    getPosts(authorID: ID!, numToFetch: Int!): [Post] @custom(http: {
        url: "https://my.api.com/person/$authorID/posts?limit=$numToFetch",
        method: GET
    })
}
```

In this case, a query like

```graphql
query {
    getPosts(authorID: "auth123", numToFetch: 10) {
        title
    }
}
```

gets transformed to an outgoing HTTP GET request to the URL `https://my.api.com/person/auth123/posts?limit=10`.

When using custom logic on fields, the URL can draw from other fields in the type.  For example:

```graphql
type User {
    username: String! @id
    ...
    posts: [Post] @custom(http: {
        url: "https://my.api.com/person/$username/posts",
        method: GET
    })
}
```

Note that:

* Fields or arguments used in the path of a URL, such as `username` or `authorID` in the exapmles above, must be marked as non-nullable (have `!` in their type); whereas, those used in parameters, such as `numToFetch`, can be nullable.
* Currently, only scalar fields or arguments are allowed to be used in URLs or bodies; though, see body below, this doesn't restrict the objects you can construct and pass to custom logic functions.
* Currently, the body can only contain alphanumeric characters in the key and other characters like `_` are not yet supported.
* Currently, constant values are not also not allowed in the body template. This would soon be supported.

## The body

Many HTTP requests, such as add and update operations on REST APIs, require a JSON formatted body to supply the data.  In a similar way to how `url` allows specifying a url pattern to use in resolving the custom request, Dgraph allows a `body` pattern that is used to build HTTP request bodies.

For example, this body can be structured JSON that relates a mutation's arguments to the JSON structure required by the remote endpoint.

```graphql
type Mutation {
    newMovie(title: String!, desc: String, dir: ID, imdb: ID): Movie @custom(http: {
            url: "http://myapi.com/movies",
            method: "POST",
            body: "{ title: $title, imdbID: $imdb, storyLine: $desc, director: { id: $dir }}",
    })
```

A request with `newMovie(title: "...", desc: "...", dir: "dir123", imdb: "tt0120316")` is transformed into a `POST` request to `http://myapi.com/movies` with a JSON body of:

```json
{
    "title": "...",
    "imdbID": "tt0120316",
    "storyLine": "...",
    "director": {
        "id": "dir123"
    }
}
```

`url` and `body` templates can be used together in a single custom definition.

For both `url` and `body` templates, any non-null arguments or fields must be present to evaluate the custom logic.  And the following rules are applied when building the request from the template for nullable arguments or fields.

* If the value of a nullable argument is present, it's used in the template.
* If a nullable argument is present, but null, then in a body `null` is inserted, while in a url nothing is added.  For example, if the `desc` argument above is null then `{ ..., storyLine: null, ...}` is constructed for the body.  Whereas, in a URL pattern like `https://a.b.c/endpoint?arg=$gqlArg`, if `gqlArg` is present, but null, the generated URL is `https://a.b.c/endpoint?arg=`.
* If a nullable argument is not present, nothing is added to the URL/body.  That would mean the constructed body would not contain `storyLine` if the `desc` argument is missing, and in `https://a.b.c/endpoint?arg=$gqlArg` the result would be `https://a.b.c/endpoint` if `gqlArg` were not present in the request arguments.

## Calling GraphQL custom resolvers

Custom queries, mutations and fields can be implemented by custom GraphQL resolvers.  In this case, use the `graphql` argument to specify which query/mutation on the remote server to call.  The syntax includes if the call is a query or mutation, the arguments, and what query/mutation to use on the remote endpoint.

For example, you can pass arguments to queries onward as arguments to remote GraphQL endpoints:

```graphql
type Query {
    getPosts(authorID: ID!, numToFetch: Int!): [Post] @custom(http: {
        url: "https://my.api.com/graphql",
        method: POST,
        graphql: "query($authorID: ID!, $numToFetch: Int!) { posts(auth: $authorID, first: $numToFetch) }"
    })
}
```

You can also define your own inputs and pass those to the remote GraphQL endpoint.

```graphql
input NewMovieInput { ... }

type Mutation {
    newMovie(input: NewMovieInput!): Movie @custom(http: {
        url: "http://movies.com/graphql",
        method: "POST",
        graphql: "mutation($input: NewMovieInput!) { addMovie(data: $input) }",
    })
```

When a schema is uploaded, Dgraph will try to introspect the remote GraphQL endpoints on any custom logic that uses the `graphql` argument.  From the results of introspection, it tries to match up arguments, input and object types to ensure that the calls to and expected responses from the remote GraphQL make sense.

If that introspection isn't possible, set `skipIntrospection: true` in the custom definition and Dgraph won't perform GraphQL schema introspection for this custom definition.

## Remote types

Any type annotated with the `@remote` directive is not stored in Dgraph.  This allows your Dgraph GraphQL instance to serve an API that includes both data stored locally and data stored or generated elsewhere.  You can also use custom fields, for example, to join data from disparate datasets.

Remote types can only be returned by custom resolvers and Dgraph won't generate any search or CRUD operations for remote types.

The schema definition used to define your Dgraph GraphQL API must include definitions of all the types used.  If a custom logic call returns a type not stored in Dgraph, then that type must be added to the Dgraph schema with the `@remote` directive.

For example, you api might use custom logic to integrate with GitHub, using either `https://api.github.com` or the GitHub GraphQL api `https://api.github.com/graphql` and calling the `user` query.  Either way, your GraphQL schema will need to include the type you expect back from that remote call.  That could be linking a `User` as stored in your Dgraph instance  with the `Repository` data from GitHub.  With `@remote` types, that's as simple as adding the type and custom call to your  schema.

```graphql
# GitHub's repository type
type Respository @remote { ... }

# Dgraph user type
type User {
    # local user name = GitHub id
    username: String! @id

    # ...
    # other data stored in Dgraph
    # ...

    # join local data with remote
    repositories: [Repository] @custom(http: {
        url:  "https://api.github.com/users/$username/repos",
        method: GET
    })
}
```

Just defining the connection is all it takes and then you can ask a single GraphQL query that performs a local query and joins with (potentialy many) remote data sources.

## How Dgraph processes custom results

Given types like

```graphql
type Post @remote {
    id: ID!
    title: String!
    datePublished: DateTime
    author: Author
}

type Author { ... }
```

and a custom query

```graphql
type Query {
    getCustomPost(id: ID!): Post @custom(http: {
        url: "https://my.api.com/post/$id",
        method: GET
    })

    getPosts(authorID: ID!, numToFetch: Int!): [Post] @custom(http: {
        url: "https://my.api.com/person/$authorID/posts?limit=$numToFetch",
        method: GET
    })
}
```

Dgraph turns the `getCustomPost` query into a HTTP request to `https://my.api.com/post/$id` and expects a single JSON object with fields `id`, `title`, `datePublished` and `author` as result.  Any additional fields are ignored, while if non-nullable fields (like `id` and `title`) are missing, GraphQL error propagation will be triggered.

For `getPosts`, Dgraph expects the HTTP call to `https://my.api.com/person/$authorID/posts?limit=$numToFetch` to return a JSON array of JSON objects, with each object matching the `Post` type as described above.

If the custom resolvers are GraphQL calls, like:

```graphql
type Query {
    getCustomPost(id: ID!): Post @custom(http: {
        url: "https://my.api.com/graphql",
        method: POST,
        graphql: "query(id: ID) { post(postID: $id) }"
    })

    getPosts(authorID: ID!, numToFetch: Int!): [Post] @custom(http: {
        url: "https://my.api.com/graphql",
        method: POST,
        graphql: "query(id: ID) { postByAuthor(authorID: $id, first: $numToFetch) }"
    })
}
```

then Dgraph expects a GraphQL call to `post` to return a valid GraphQL result like `{ "data": { "post": {...} } }` and will use the JSON object that is the value of `post` as the data resolved by the request.

Similarly, Dgraph expects `postByAuthor` to return data like `{ "data": { "postByAuthor": [ {...}, ... ] } }` and will use the array value of `postByAuthor` to build its array of posts result.


## How custom fields are resolved

When evaluating a request that includes custom fields, Dgraph might run multiple resolution stages to resolve all the fields.  Dgraph must also ensure it requests enough data to forfull the custom fields.  For example, given the `User` type defined as:

```graphql
type User {
    username: String! @id
    ...
    posts: [Post] @custom(http: {
        url: "https://my.api.com/person/$username/posts",
        method: GET
    })
}
```

a query such as:

```graphql
query {
    queryUser {
        username
        posts
    }
}
```

is executed by first querying in Dgraph for `username` and then using the result to resolve the custom field `posts` (which relies on `username`).  For a request like:

```graphql
query {
    queryUser {
        posts
    }
}
```

Dgraph works out that it must first get `username` so it can run the custom field `posts`, even though `username` isn't part of the original query.  So Dgraph retrieves enough data to satisfy the custom request, even if that involves data that isn't asked for in the query.

There are currently a few limitations on custom fields:

* each custom call must include either an `ID` or `@id` field
* arguments are not allowed (soon custom field arguments will be allowed and will be used in the `@custom` directive in the same manner as for custom queries and mutations), and
* a custom field can't depend on another custom field (longer term, we intend to lift this restriction).

## Restrictions / Roadmap

Our custom logic is still in beta and we are improving it quickly.  Here's a few points that we plan to work on soon:

* adding arguments to custom fields
* relaxing the restrictions on custom fields using id values
* iterative evaluation of `@custom` and `@remote` - in the current version you can't have `@custom` inside an `@remote` type once we add this, you'll be able to extend remote types with custom fields, and
* allowing fine tuning of the generated API, for example removing of customizing the generated CRUD mutations.

---
