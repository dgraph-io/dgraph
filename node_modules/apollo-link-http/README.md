---
title: apollo-link-http
description: Get GraphQL results over a network using HTTP fetch.
---

The http link is the most common Apollo Link, a system of modular components
for GraphQL networking. If you haven't done so already, [read the Apollo Link
docs](https://www.apollographql.com/docs/link/#usage) to learn about the Apollo
Link ecosystem and how to use this link with libraries like Apollo Client and
graphql-tools, or as a standalone client.

The http link is a terminating link that fetches GraphQL results from a GraphQL
endpoint over an http connection. The http link supports both POST and GET
requests with the ability to change the http options on a per query basis. This
can be used for authentication, persisted queries, dynamic uris, and other
granular updates.

## Usage

Import and initialize this link in just two lines:

```js
import { createHttpLink } from "apollo-link-http";

const link = createHttpLink({ uri: "/graphql" });
```

## Options

HTTP Link takes an object with some options on it to customize the behavior of the link. If your server supports it, the HTTP link can also send over metadata about the request in the extensions field. To enable this, pass `includeExtensions` as true. The options you can pass are outlined below:

* `uri`: the URI key is a string endpoint or function resolving to an endpoint -- will default to "/graphql" if not specified
* `includeExtensions`: allow passing the extensions field to your graphql server, defaults to false
* `fetch`: a `fetch` compatible API for making a request
* `headers`: an object representing values to be sent as headers on the request
* `credentials`: a string representing the credentials policy you want for the fetch call. Possible values are: `omit`, `include` and `same-origin`
* `fetchOptions`: any overrides of the fetch options argument to pass to the fetch call
* `useGETForQueries`: set to `true` to use the HTTP `GET` method for queries (but not for mutations)

## Fetch polyfill

The HTTP Link relies on having `fetch` present in your runtime environment. If you are running on react-native, or modern browsers, this should be no problem. If you are targeting an environment without `fetch` such as older browsers or the server, you will need to pass your own `fetch` to the link through the options. We recommend [`unfetch`](https://github.com/developit/unfetch) for older browsers and [`node-fetch`](https://github.com/bitinn/node-fetch) for running in Node.

## Context

The Http Link uses the `headers` field on the context to allow passing headers to the HTTP request. It also supports the `credentials` field for defining credentials policy, `uri` for changing the endpoint dynamically, and `fetchOptions` to allow generic fetch overrides (i.e. `method: "GET"`). These options will override the same key if passed when creating the the link.

Note that if you set `fetchOptions.method` to `GET`, the http link will follow the [standard GraphQL HTTP GET encoding](http://graphql.org/learn/serving-over-http/#get-request): the query, variables, operation name, and extensions will be passed as query parameters rather than in the HTTP request body. If you want mutations to continue to be sent as non-idempotent `POST` requests, set the top-level `useGETForQueries` option to `true` instead of setting `fetchOptions.method` to `GET`.

This link also attaches the response from the `fetch` operation on the context as `response` so you can access it from within another link.

* `headers`: an object representing values to be sent as headers on the request
* `credentials`: a string representing the credentials policy you want for the fetch call. Possible values are: `omit`, `include` and `same-origin`
* `uri`: a string of the endpoint you want to fetch from
* `fetchOptions`: any overrides of the fetch options argument to pass to the fetch call
* `response`: this is the raw response from the fetch request after it is made.
* `http`: this is an object to control fine grained aspects of the http link itself, such as persisted queries (see below)

### Persisted queries

The http link supports an advanced GraphQL feature called persisted queries. This allows you to not send the stringified query over the wire, but instead send some kind of identifier of the query. To support this you need to attach the id somewhere to the extensions field and pass the following options to the context:

```js
operation.setContext({
  http: {
    includeExtensions: true,
    includeQuery: false,
  }
})
```

The `http` object on context currently supports two keys:

* `includeExtensions`: Send the extensions object for this request.
* `includeQuery`: Don't send the `query` field for this request.

One way to use persisted queries is with [apollo-link-persisted-queries](https://github.com/apollographql/apollo-link-persisted-queries) and [Apollo Engine](https://www.apollographql.com/docs/engine/auto-persisted-queries.html).

### Passing context per query

Apollo Client supports passing context separately for every query, so you can do things like pass a special header for a single query invocation if you need to.

```js
import { createHttpLink } from "apollo-link-http";
import ApolloClient from "apollo-client";
import { InMemoryCache } from "apollo-cache-inmemory";

const client = new ApolloClient({
  link: createHttpLink({ uri: "/graphql" }),
  cache: new InMemoryCache()
});

// a query with apollo-client
client.query({
  query: MY_QUERY,
  context: {
    // example of setting the headers with context per operation
    headers: {
      special: "Special header value"
    }
  }
});
```

## Errors

The Http Link draws a distinction between client, server and GraphQL errors. Server errors can occur in three different scenarios: parse, network and data errors. [`apollo-link-error`](error) provides an [interface](error#callback) for handling these errors. This list describes the scenarios that cause different errors:

* _Client parse error_: the request body is not-serializable due to circular references for example
* _Server parse error_: the response from the server cannot be parsed ([response.json()](https://developer.mozilla.org/en-US/docs/Web/API/Body/json))
* _Server network error_: the response has a status of >= 300
* _Server data error_: the parse request does not contain `data` or `errors`
* _GraphQL error_: an objects in the `errors` array for a 200 level status

Since many server implementations can return a valid GraphQL result on a server network error, the thrown `Error` object contains the parsed server result. A server data error also receives the parsed result.

The table below provides a summary of error, `Observable` method called by the HTTP link, and type of error thrown for each failure:

| Error          | Callback | Error Type         |
| -------------- | :------: | ------------------ |
| Client Parse   | `error`  | `ClientParseError` |
| Server Parse   | `error`  | `ServerParseError` |
| Server Network | `error`  | `ServerError`      |
| Server Data    | `error`  | `ServerError`      |
| GraphQL Error  |  `next`  | `Object`           |

All error types inherit the `name`, `message`, and nullable `stack` properties from the generic javascript [Error](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Error).

```js
//type ClientParseError
{
  parseError: Error;                // Error returned from response.json()
};

//type ServerParseError
{
  response: Response;               // Object returned from fetch()
  statusCode: number;               // HTTP status code
  bodyText: string                  // text that was returned from server
};

//type ServerError
{
  result: Record<string, any>;      // Parsed object from server response
  response: Response;               // Object returned from fetch()
  statusCode: number;               // HTTP status code
};
```

## Custom fetching

You can use the `fetch` option when creating an http-link to do a lot of custom networking. This is useful if you want to modify the request based on the calculated headers  or calculate the uri based on the operation:

### Custom auth

```js
const customFetch = (uri, options) => {
  const { header } = Hawk.client.header(
    "http://example.com:8000/resource/1?b=1&a=2",
    "POST",
    { credentials: credentials, ext: "some-app-data" }
  );
  options.headers.Authorization = header;
  return fetch(uri, options);
};

const link = createHttpLink({ fetch: customFetch });
```

### Dynamic URI

```js
const customFetch = (uri, options) => {
  const { operationName } = JSON.parse(options.body);
  return fetch(`${uri}/graph/graphql?opname=${operationName}`, options);
};

const link = createHttpLink({ fetch: customFetch });
```

## Upgrade: Apollo Client 1.0

If you previously used either `apollo-fetch` or `apollo-client`'s `createNetworkInterface`, you will need to change the way `use` and `useAfter` are implemented in your app. Both can be implemented by writing a custom link. It's important to note that regardless of whether you're adding middleware or afterware, your Http link will always be last in the chain since it's a terminating link.

#### Middleware

_Before_

```js
// before
import ApolloClient, { createNetworkInterface } from "apollo-client";

const networkInterface = createNetworkInterface({ uri: "/graphql" });

networkInterface.use([
  {
    applyMiddleware(req, next) {
      if (!req.options.headers) {
        req.options.headers = {}; // Create the header object if needed.
      }
      req.options.headers["authorization"] = localStorage.getItem("token")
        ? localStorage.getItem("token")
        : null;
      next();
    }
  }
]);
```

_After_

```js
import { ApolloLink } from "apollo-link";
import { createHttpLink } from "apollo-link-http";

const httpLink = createHttpLink({ uri: "/graphql" });
const middlewareLink = new ApolloLink((operation, forward) => {
  operation.setContext({
    headers: {
      authorization: localStorage.getItem("token") || null
    }
  });
  return forward(operation);
});

// use with apollo-client
const link = middlewareLink.concat(httpLink);
```

#### Afterware (error)

_Before_

```js
import ApolloClient, { createNetworkInterface } from "apollo-client";
import { logout } from "./logout";

const networkInterface = createNetworkInterface({ uri: "/graphql" });

networkInterface.useAfter([
  {
    applyAfterware({ response }, next) {
      if (response.statusCode === 401) {
        logout();
      }
      next();
    }
  }
]);
```

_After_

```js
import { ApolloLink } from "apollo-link";
import { createHttpLink } from "apollo-link-http";
import { onError } from "apollo-link-error";

import { logout } from "./logout";

const httpLink = createHttpLink({ uri: "/graphql" });
const errorLink = onError(({ networkError }) => {
  if (networkError.statusCode === 401) {
    logout();
  }
});

// use with apollo-client
const link = errorLink.concat(httpLink);
```

#### Afterware (data manipulation)

_Before_

```js
import ApolloClient, { createNetworkInterface } from "apollo-client";

const networkInterface = createNetworkInterface({ uri: "/graphql" });

networkInterface.useAfter([
  {
    applyAfterware({ response }, next) {
      if (response.data.user.lastLoginDate) {
        response.data.user.lastLoginDate = new Date(
          response.data.user.lastLoginDate
        );
      }
      next();
    }
  }
]);
```

_After_

```js
import { ApolloLink } from "apollo-link";
import { createHttpLink } from "apollo-link-http";

const httpLink = createHttpLink({ uri: "/graphql" });
const addDatesLink = new ApolloLink((operation, forward) => {
  return forward(operation).map(response => {
    if (response.data.user.lastLoginDate) {
      response.data.user.lastLoginDate = new Date(
        response.data.user.lastLoginDate
      );
    }
    return response;
  });
});

// use with apollo-client
const link = addDatesLink.concat(httpLink);
```
