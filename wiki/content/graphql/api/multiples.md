+++
title = "Multiple GraphQL Operations in a Request"
weight = 5
[menu.main]
    parent = "api"
    name = "Multiple GraphQL Operations in a Request"
+++

GraphQL requests can contain one or more operations. Operations are one of `query`, `mutation`, or `subscription`. If a request only has one operation, then it can be unnamed like the following:

## Single Operation

The most basic request contains a single anonymous (unnamed) operation. Each operation can have one or more queries within in. For example, the following query has `query` operation running the queries "getTask" and "getUser":

```graphql
query {
  getTask(id: "0x3") {
    id
    title
    completed
  }
  getUser(username: "dgraphlabs") {
    username
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
      "completed": true
    },
    "getUser": {
      "username": "dgraphlabs"
    }
  }
}
```

You can optionally name the operation as well, though it's not required if the request only has one operation as it's clear what needs to be executed.

### Query Shorthand

If a request only has a single query operation, then you can use the short-hand form of omitting the "query" keyword:

```graphql
{
  getTask(id: "0x3") {
    id
    title
    completed
  }
  getUser(username: "dgraphlabs") {
    username
  }
}
```

This simplfies queries when a query doesn't require an operation name or [variables](/graphql/api/variables).

## Multiple Operations

If a request has two or more operations, then each operation must have a name. A request can only execute one operation, so you must also include the operation name to execute in the request (see the "operations" field for [requests](/graphql/api/requests)). Every operation name in a request must be unique.

For example, in the following request has the operation names "getTaskAndUser" and "completedTasks".

```graphql
query getTaskAndUser {
  getTask(id: "0x3") {
    id
    title
    completed
  }
  queryUser(filter: {username: {eq: "dgraphlabs"}}) {
    username
    name
  }
}

query completedTasks {
  queryTask(filter: {completed: true}) {
    title
    completed
  }
}
```

When executing the following request (as an HTTP POST request in JSON format), specifying the "getTaskAndUser" operation executes the first query:

```json
{
  "query": "query getTaskAndUser { getTask(id: \"0x3\") { id title completed } queryUser(filter: {username: {eq: \"dgraphlabs\"}}) { username name }\n}\n\nquery completedTasks { queryTask(filter: {completed: true}) { title completed }}",
  "operationName": "getTaskAndUser"
}
```

```json
{
  "data": {
    "getTask": {
      "id": "0x3",
      "title": "GraphQL docs example",
      "completed": true
    },
    "queryUser": [
      {
        "username": "dgraphlabs",
        "name": "Dgraph Labs"
      }
    ]
  }
}
```

And specifying the "completedTasks" operation executes the second query:

```json
{
	"query": "query getTaskAndUser { getTask(id: \"0x3\") { id title completed } queryUser(filter: {username: {eq: \"dgraphlabs\"}}) { username name }\n}\n\nquery completedTasks { queryTask(filter: {completed: true}) { title completed }}",
	"operationName": "completedTasks"
}
```

```json
{
  "data": {
    "queryTask": [
      {
        "title": "GraphQL docs example",
        "completed": true
      },
      {
        "title": "Show second operation",
        "completed": true
      }
    ]
  }
}
```

## Additional Details

When an operation contains multiple queries, they are run concurrently and independently in a  Dgraph readonly transaction per query.

When an operation contains multiple mutations, they are run serially, in the order listed in the request, and in a transaction per mutation. If a mutation fails, the following mutations are not executed, and previous mutations are not rolled back.
