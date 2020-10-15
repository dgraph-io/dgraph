+++
title = "Requests and Responses"
weight = 2
[menu.main]
    parent = "api"
    name = "Requests and Responses"
+++

In this section, we'll cover the structure for GraphQL requests and responses, how to enable compression for them, and configuration options for extensions.

## Requests

GraphQL requests can be sent via HTTP POST or HTTP GET requests.

POST requests sent with the Content-Type header `application/graphql` must have a POST body content as a GraphQL query string. For example, the following is a valid POST body for a query:

```graphql
query {
  getTask(id: "0x3") {
    id
    title
    completed
    user {
      username
      name
    }
  }
}
```

POST requests sent with the Content-Type header `application/json` must have a POST body in the following JSON format:

```json
{
  "query": "...",
  "operationName": "...",
  "variables": { "var": "val", ... }
}
```

GET requests must be sent in the following format. The query, variables, and operation are sent as URL-encoded query parameters in the URL.

```
http://localhost:8080/graphql?query={...}&variables={...}&operation=...
```

In either request method (POST or GET), only `query` is required. `variables` is only required if the query contains GraphQL variables: i.e. the query starts like `query myQuery($var: String)`. `operationName` is only required if there are multiple operations in the query; in which case, operations must also be named.

## Responses

GraphQL responses are in JSON. Every response is a JSON map, and will include JSON keys for `"data"`, `"errors"`, or `"extensions"` following the GraphQL specification. They follow the following formats.

Successful queries are in the following format:

```json
{
  "data": { ... },
  "extensions": { ... }
}
```

Queries that have errors are in the following format.

```json
{
  "errors": [ ... ],
}
```

All responses, including errors, always return HTTP 200 OK status codes. An error response will contain an `"errors"` field.

### "data" field

The "data" field contains the result of your GraphQL request. The response has exactly the same shape as the result. For example, notice that for the following query, the response includes the data in the exact shape as the query.

Query:

```graphql
query {
  getTask(id: "0x3") {
    id
    title
    completed
    user {
      username
      name
    }
  }
}
```

Response:

```json
{
  "data": {
    "getTask": {
      "id": "0x3",
      "title": "GraphQL docs example",
      "completed": true,
      "user": {
        "username": "dgraphlabs",
        "name": "Dgraph Labs"
      }
    }
  }
}
```

### "errors" field

The "errors" field is a JSON list where each entry has a `"message"` field that describes the error and optionally has a `"locations"` array to list the specific line and column number of the request that points to the error described. For example, here's a possible error for the following query, where `getTask` needs to have an `id` specified as input:

Query:
```graphql
query {
  getTask() {
    id
  }
}
```

Response:
```json
{
  "errors": [
    {
      "message": "Field \"getTask\" argument \"id\" of type \"ID!\" is required but not provided.",
      "locations": [
        {
          "line": 2,
          "column": 3
        }
      ]
    }
  ]
}
```

### "extensions" field

The "extensions" field contains extra metadata for the request with metrics and trace information for the request.

- `"touched_uids"`: The number of nodes that were touched to satisfy the request. This is a good metric to gauge the complexity of the query.
- `"tracing"`: Displays performance tracing data in [Apollo Tracing][apollo-tracing] format. This includes the duration of the whole query and the duration of each operation.

[apollo-tracing]: https://github.com/apollographql/apollo-tracing

Here's an example of a query response with the extensions field:

```json
{
  "data": {
    "getTask": {
      "id": "0x3",
      "title": "GraphQL docs example",
      "completed": true,
      "user": {
        "username": "dgraphlabs",
        "name": "Dgraph Labs"
      }
    }
  },
  "extensions": {
    "touched_uids": 9,
    "tracing": {
      "version": 1,
      "startTime": "2020-07-29T05:54:27.784837196Z",
      "endTime": "2020-07-29T05:54:27.787239465Z",
      "duration": 2402299,
      "execution": {
        "resolvers": [
          {
            "path": [
              "getTask"
            ],
            "parentType": "Query",
            "fieldName": "getTask",
            "returnType": "Task",
            "startOffset": 122073,
            "duration": 2255955,
            "dgraph": [
              {
                "label": "query",
                "startOffset": 171684,
                "duration": 2154290
              }
            ]
          }
        ]
      }
    }
  }
}
```

### Turn off extensions

Extensions are returned in every response. These are completely optional. If you'd like to turn off extensions, you can set the config option `--graphql_extensions=false` in Dgraph Alpha.

## Compression

By default, requests and responses are not compressed. Typically, enabling compression saves from sending additional data to and from the backend while using a bit of extra processing time to do the compression.

You can turn on compression for requests and responses by setting the standard HTTP headers. To send compressed requests, set HTTP header `Content-Encoding` to `gzip` to POST gzip-compressed data. To receive compressed responses, set the HTTP header `Accept-Encoding` to `gzip` in your request.
