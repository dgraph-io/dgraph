+++
date = "2017-03-20T22:25:17+11:00"
title = "Conditional Upsert"
weight = 12
[menu.main]
    parent = "mutations"
+++

The upsert block also allows specifying conditional mutation blocks using an `@if`
directive. The mutation is executed only when the specified condition is true. If the
condition is false, the mutation is silently ignored. The general structure of
Conditional Upsert looks like as follows:

```
upsert {
  query <query block>
  [fragment <fragment block>]
  mutation [@if(<condition>)] <mutation block 1>
  [mutation [@if(<condition>)] <mutation block 2>]
  ...
}
```

The `@if` directive accepts a condition on variables defined in the query block and can be
connected using `AND`, `OR` and `NOT`.

## Example of Conditional Upsert

Let's say in our previous example, we know the `company1` has less than 100 employees.
For safety, we want the mutation to execute only when the variable `v` stores less than
100 but greater than 50 UIDs in it. This can be achieved as follows:

```sh
curl -H "Content-Type: application/rdf" -X POST localhost:8080/mutate?commitNow=true -d  $'
upsert {
  query {
    v as var(func: regexp(email, /.*@company1.io$/))
  }

  mutation @if(lt(len(v), 100) AND gt(len(v), 50)) {
    delete {
      uid(v) <name> * .
      uid(v) <email> * .
      uid(v) <age> * .
    }
  }
}' | jq
```

We can achieve the same result using `json` dataset as follows:

```sh
curl -H "Content-Type: application/json" -X POST localhost:8080/mutate?commitNow=true -d '{
  "query": "{ v as var(func: regexp(email, /.*@company1.io$/)) }",
  "cond": "@if(lt(len(v), 100) AND gt(len(v), 50))",
  "delete": {
    "uid": "uid(v)",
    "name": null,
    "email": null,
    "age": null
  }
}' | jq
```

## Example of Multiple Mutation Blocks

Consider an example with the following schema:

```sh
curl localhost:8080/alter -X POST -d $'
  name: string @index(term) .
  email: [string] @index(exact) @upsert .' | jq
```

Let's say, we have many users stored in our database each having one or more than
one email Addresses. Now, we get two email Addresses that belong to the same user.
If the email Addresses belong to the different nodes in the database, we want to delete
the existing nodes and create a new node with both the emails attached to this new node.
Otherwise, we create/update the new/existing node with both the emails.

```sh
curl -H "Content-Type: application/rdf" -X POST localhost:8080/mutate?commitNow=true -d $'
upsert {
  query {
    # filter is needed to ensure that we do not get same UIDs in u1 and u2
    q1(func: eq(email, "user_email1@company1.io")) @filter(not(eq(email, "user_email2@company1.io"))) {
      u1 as uid
    }

    q2(func: eq(email, "user_email2@company1.io")) @filter(not(eq(email, "user_email1@company1.io"))) {
      u2 as uid
    }

    q3(func: eq(email, "user_email1@company1.io")) @filter(eq(email, "user_email2@company1.io")) {
      u3 as uid
    }
  }

  # case when both emails do not exist
  mutation @if(eq(len(u1), 0) AND eq(len(u2), 0) AND eq(len(u3), 0)) {
    set {
      _:user <name> "user" .
      _:user <dgraph.type> "Person" .
      _:user <email> "user_email1@company1.io" .
      _:user <email> "user_email2@company1.io" .
    }
  }

  # case when email1 exists but email2 does not
  mutation @if(eq(len(u1), 1) AND eq(len(u2), 0) AND eq(len(u3), 0)) {
    set {
      uid(u1) <email> "user_email2@company1.io" .
    }
  }

  # case when email1 does not exist but email2 exists
  mutation @if(eq(len(u1), 0) AND eq(len(u2), 1) AND eq(len(u3), 0)) {
    set {
      uid(u2) <email> "user_email1@company1.io" .
    }
  }

  # case when both emails exist and needs merging
  mutation @if(eq(len(u1), 1) AND eq(len(u2), 1) AND eq(len(u3), 0)) {
    set {
      _:user <name> "user" .
      _:user <dgraph.type> "Person" .
      _:user <email> "user_email1@company1.io" .
      _:user <email> "user_email2@company1.io" .
    }

    delete {
      uid(u1) <name> * .
      uid(u1) <email> * .
      uid(u2) <name> * .
      uid(u2) <email> * .
    }
  }
}' | jq
```

Result (when database is empty):

```json
{
  "data": {
    "q1": [],
    "q2": [],
    "q3": [],
    "code": "Success",
    "message": "Done",
    "uids": {
      "user": "0x1"
    }
  },
  "extensions": {...}
}
```

Result (both emails exist and are attached to different nodes):
```json
{
  "data": {
    "q1": [
      {
        "uid": "0x2"
      }
    ],
    "q2": [
      {
        "uid": "0x3"
      }
    ],
    "q3": [],
    "code": "Success",
    "message": "Done",
    "uids": {
      "user": "0x4"
    }
  },
  "extensions": {...}
}
```

Result (when both emails exist and are already attached to the same node):

```json
{
  "data": {
    "q1": [],
    "q2": [],
    "q3": [
      {
        "uid": "0x4"
      }
    ],
    "code": "Success",
    "message": "Done",
    "uids": {}
  },
  "extensions": {...}
}
```

We can achieve the same result using `json` dataset as follows:

```sh
curl -H "Content-Type: application/json" -X POST localhost:8080/mutate?commitNow=true -d '{
  "query": "{q1(func: eq(email, \"user_email1@company1.io\")) @filter(not(eq(email, \"user_email2@company1.io\"))) {u1 as uid} \n q2(func: eq(email, \"user_email2@company1.io\")) @filter(not(eq(email, \"user_email1@company1.io\"))) {u2 as uid} \n q3(func: eq(email, \"user_email1@company1.io\")) @filter(eq(email, \"user_email2@company1.io\")) {u3 as uid}}",
  "mutations": [
    {
      "cond": "@if(eq(len(u1), 0) AND eq(len(u2), 0) AND eq(len(u3), 0))",
      "set": [
        {
          "uid": "_:user",
          "name": "user",
          "dgraph.type": "Person"
        },
        {
          "uid": "_:user",
          "email": "user_email1@company1.io",
          "dgraph.type": "Person"
        },
        {
          "uid": "_:user",
          "email": "user_email2@company1.io",
          "dgraph.type": "Person"
        }
      ]
    },
    {
      "cond": "@if(eq(len(u1), 1) AND eq(len(u2), 0) AND eq(len(u3), 0))",
      "set": [
        {
          "uid": "uid(u1)",
          "email": "user_email2@company1.io",
          "dgraph.type": "Person"
        }
      ]
    },
    {
      "cond": "@if(eq(len(u1), 1) AND eq(len(u2), 0) AND eq(len(u3), 0))",
      "set": [
        {
          "uid": "uid(u2)",
          "email": "user_email1@company1.io",
          "dgraph.type": "Person"
        }
      ]
    },
    {
      "cond": "@if(eq(len(u1), 1) AND eq(len(u2), 1) AND eq(len(u3), 0))",
      "set": [
        {
          "uid": "_:user",
          "name": "user",
          "dgraph.type": "Person"
        },
        {
          "uid": "_:user",
          "email": "user_email1@company1.io",
          "dgraph.type": "Person"
        },
        {
          "uid": "_:user",
          "email": "user_email2@company1.io",
          "dgraph.type": "Person"
        }
      ],
      "delete": [
        {
          "uid": "uid(u1)",
          "name": null,
          "email": null
        },
        {
          "uid": "uid(u2)",
          "name": null,
          "email": null
        }
      ]
    }
  ]
}' | jq
```
