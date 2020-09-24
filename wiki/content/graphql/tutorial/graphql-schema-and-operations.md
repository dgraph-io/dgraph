+++
title = "GraphQL"
[menu.main]
    parent = "build-an-app-tutorial"
    identifier = "graphql-schema-and-operations"
    weight = 4   
+++


Schema and operations... follow on from our design


## GraphQL schema

maybe this all moves to another section ??? yeah I think one just on GraphQL and operations
FIXME: to move

This is SDL & GraphQL focused

```graphql
type Task {
    ...
}

type User {
    ...
}
```

## GraphQL Operations

### mutations


```graphql
mutation {
  addUser(input: [
    {
      username: "alice@dgraph.io",
      name: "Alice",
      tasks: [
        {
          title: "Avoid touching your face",
          completed: false,
        },
        {
          title: "Stay safe",
          completed: false
        },
        {
          title: "Avoid crowd",
          completed: true,
        },
        {
          title: "Wash your hands often",
          completed: true
        }
      ]
    }
  ]) {
    user {
      username
      name
      tasks {
        id
        title
      }
    }
  }
}
```

### queries

```graphql
query {
  queryTask {
    id
    title
    completed
    user {
        username
    }
  }
}
```

Running the query above should return JSON response as shown below:

```json
{
  "data": {
    "queryTask": [
      {
        "id": "0x3",
        "title": "Avoid touching your face",
        "completed": false,
        "user": {
          "username": "alice@dgraph.io"
        }
      },
      {
        "id": "0x4",
        "title": "Stay safe",
        "completed": false,
        "user": {
          "username": "alice@dgraph.io"
        }
      },
      {
        "id": "0x5",
        "title": "Avoid crowd",
        "completed": true,
        "user": {
          "username": "alice@dgraph.io"
        }
      },
      {
        "id": "0x6",
        "title": "Wash your hands often",
        "completed": true,
        "user": {
          "username": "alice@dgraph.io"
        }
      }
    ]
  }
}
```


#### Querying Data with Filters

Before we get into querying data with filters, we will be required
to define search indexes to the specific fields.

Let's say we have to run a query on the _completed_ field, for which
we add `@search` directive to the field, as shown in the schema below:

```graphql
type Task {
  id: ID!
  title: String!
  completed: Boolean! @search
  user: User!
}
```

The `@search` directive is added to support the native search indexes of **Dgraph**.

Resubmit the updated schema -
```
curl -X POST localhost:8080/admin/schema --data-binary '@schema.graphql'
```

Now, let's fetch all todos which are completed :

```graphql
query {
  queryTask(filter: {
    completed: true
  }) {
    title
    completed
  }
}
```

Next, let's say we have to run a query on the _title_ field, for which
we add another `@search` directive to the field, as shown in the schema below:

```graphql
type Task {
    id: ID!
    title: String! @search(by: [fulltext])
    completed: Boolean! @search
    user: User!
}
```

The `fulltext` search index provides the advanced search capability to perform equality
comparison as well as matching with language-specific stemming and stopwords.

Resubmit the updated schema -
```
curl -X POST localhost:8080/admin/schema --data-binary '@schema.graphql'
```

Now, let's try to fetch todos whose title has the word _"avoid"_ :

```graphql
query {
  queryTask(filter: {
    title: {
      alloftext: "avoid"
    }
  }) {
    id
    title
    completed
  }
}
```


### subscriptions

## What's next
