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

## Reverse Edges

Any outgoing edge in Dgraph can be reversed using the `@reverse` directive in Schema and be queried using tilde as the prefix of the reverse edge. e.g. `<~myEdge>`.

Dgraph serializes directed graphs. This means that all properties always point from an entity to another entity or value in a single direction. `S P -> O`.

However, in some cases, users want to create an entity in the reverse direction. But there is no specific syntax for reverse edges in Dgraph mutations. Because Reverse edges are just a kind of edge indexing. It is not literally an edge, but a kind of "pointer" and it will always be represented as `S P -> O` in query responses. But the "tilde" sets what is the direction for the user.

There are some confusions about syntax related to reverse e.g.

> The below datasets are wrong.
```RDF
_:BlankNode <~myEdge> _:MyObject . #That's really wrong syntax.
_:BlankNode <dgraph.type> "Person" .
```
or

```JSON
{
   "set": [
      {
         "uid": "_:BlankNode",
         "dgraph.type": "Person",
         "~myEdge": {
            "uid": "_:MyObject",
            "dgraph.type": "Object"
         }
      }
   ]
}
```

### Using Reverse Edges correctly

Fixing the previous RDF mutation:

In RDF the arrangement of the triples already defines what can be reverse.

```RDF
_:MyObject <myEdge> _:BlankNode  . #That's the right syntax of a reverse edge.
_:BlankNode <dgraph.type> "Person" .
```

Fixing the previous JSON mutation:

The easiest way to correct and apply reverse edges using JSON. It is simply having the directive on schema at the desired edge and when building your mutations remember that there is no reverse syntax in JSON. So what you should do is similar to RDF. Just change the arrangement of the JSON objects.

Since `MyObject` is above the `Person` entity. So `MyObject` must come before when formatting the mutation.

```JSON
{
   "set": [
      {
         "uid": "_:MyObject",
         "dgraph.type": "Object",
         "myEdge": {
            "uid": "_:BlankNode",
            "dgraph.type": "Person"
         }
      }
   ]
}
```

Another way to do this is to separate into small chunks/batches and use blank nodes as references. This facilitates the organization and reuse of references.

```JSON
{
   "set": [
      {
         "uid": "_:MyObject",
         "dgraph.type": "Object",
         "myEdge": [{"uid": "_:BlankNode"}]
      },
      {
         "uid": "_:BlankNode",
         "dgraph.type": "Person"
      }
   ]
}
```

### More reverse examples

In RDF the correct way to apply reverse edges is very straightforward.

```RDF
name: String .
husband: uid @reverse .
wife: uid @reverse .
parent: [uid] @reverse .
```

```RDF
{
  set {
    _:Megalosaurus <name> "Earl Sneed Sinclair" .
    _:Megalosaurus <dgraph.type> "Dinosaur" .
    _:Megalosaurus <wife> _:Allosaurus .
    _:Allosaurus <name> "Francis Johanna Phillips Sinclair" (short="Fran") .
    _:Allosaurus <dgraph.type> "Dinosaur" .
    _:Allosaurus <husband> _:Megalosaurus .
    _:Hypsilophodon <name> "Robert Mark Sinclair" (short="Robbie") .
    _:Hypsilophodon <dgraph.type> "Dinosaur" .
    _:Hypsilophodon <parent> _:Allosaurus (role="son") .
    _:Hypsilophodon <parent> _:Megalosaurus (role="son") .
    _:Protoceratops <name> "Charlene Fiona Sinclair" .
    _:Protoceratops <dgraph.type> "Dinosaur" .
    _:Protoceratops <parent> _:Allosaurus (role="daughter") .
    _:Protoceratops <parent> _:Megalosaurus (role="daughter") .
    _:MegalosaurusBaby <name> "Baby Sinclair" (short="Baby") .
    _:MegalosaurusBaby <dgraph.type> "Dinosaur" .
    _:MegalosaurusBaby <parent> _:Allosaurus (role="son") .
    _:MegalosaurusBaby <parent> _:Megalosaurus (role="son") .
  }
}
```

The directions are like:

```
Exchanged hierarchy:
 Object -> Parent;
 Object <~ Parent; #Reverse
 Children to parents via "parent" edge.
 wife and husband bidirectional using reverse.
Normal hierarchy:
 Parent -> Object;
 Parent <~ Object; #Reverse
 This hierarchy is not part of the example, but is generally used in all graph models.
 To make this hierarchy we need to bring the hierarchical relationship starting from the parents and not from the children. Instead of using the edges "wife" and "husband" we switch to single edge called "married" to simplify the model.
    _:Megalosaurus <name> "Earl Sneed Sinclair" .
    _:Megalosaurus <dgraph.type> "Dinosaur" .
    _:Megalosaurus <married> _:Allosaurus .
    _:Megalosaurus <parent> _:Hypsilophodon (role="son") .
    _:Megalosaurus <parent> _:Protoceratops (role="daughter") .
    _:Megalosaurus <parent> _:MegalosaurusBaby (role="son") .
    _:Allosaurus <name> "Francis Johanna Phillips Sinclair" (short="Fran") .
    _:Allosaurus <dgraph.type> "Dinosaur" .
    _:Allosaurus <married> _:Megalosaurus .
    _:Allosaurus <parent> _:Hypsilophodon (role="son") .
    _:Allosaurus <parent> _:Protoceratops (role="daughter") .
    _:Allosaurus <parent> _:MegalosaurusBaby (role="son") .
```

### Queries

1. `wife_husband` is the reversed `wife` edge.
2. `husband` is an actual edge.

```
{
  q(func: has(wife)) {
    name
    WF as wife {
      name
    }
  }
  reverseIt(func: uid(WF)) {
    name
    wife_husband : ~wife {
      name
    }
    husband {
      name
    }
  }
}
```

1. `Children` is the reversed `parent` edge.

```
{
  q(func: has(name)) @filter(eq(name, "Earl Sneed Sinclair")){
    name
    Children : ~parent @facets {
      name
    }
  }
}
```

## Reverse Edges and Facets

Facets on reverse edges have their direction preserved. That is, if you create a facet from child to parent and another facet from parent to child, both will coexist independently.

```
{
  set {
    _:Megalosaurus <name> "Earl Sneed Sinclair" .
    _:Megalosaurus <dgraph.type> "Dinosaur" .
    _:Megalosaurus <wife> _:Allosaurus .
    _:Megalosaurus <parent> _:MegalosaurusBaby (role="parent -> child") .
    _:MegalosaurusBaby <name> "Baby Sinclair" (short="Baby -> parent") .
    _:MegalosaurusBaby <dgraph.type> "Dinosaur" .
    _:MegalosaurusBaby <parent> _:Megalosaurus (role="child -> parent") .
  }
}
```

Using a similar query from the previous example:

```
{
  Parent(func: has(name)) @filter(eq(name, "Earl Sneed Sinclair")){
    name
    C as Children : parent @facets {
      name
    }
  }
    Child(func: uid(C)) {
      name
      parent @facets {
        name
      }
    }
}
```

```
{
  "data": {
    "Parent": [
      {
        "name": "Earl Sneed Sinclair",
        "Children": [
          {
            "name": "Baby Sinclair",
            "Children|role": "parent -> child"
          }
        ]
      }
    ],
    "Child": [
      {
        "name": "Baby Sinclair",
        "parent": [
          {
            "name": "Earl Sneed Sinclair",
            "parent|role": "child -> parent"
          }
        ]
      }
    ]
  }
}
 ```
 