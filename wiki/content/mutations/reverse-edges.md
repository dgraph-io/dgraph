+++
date = "2017-03-20T22:25:17+11:00"
title = "Reverse Edges"
weight = 13
[menu.main]
    parent = "mutations"
+++

Any outgoing edge in Dgraph can be reversed using the `@reverse` directive in the schema and be queried using tilde as the prefix of the edge name. e.g. `<~myEdge>`.

Dgraph serializes directed graphs. This means that all properties always point from an entity to another entity or value in a single direction. `S P -> O`.

Reverse edges are automatically generated edges and are not part of your dataset. This means that you cannot run mutations directly on the reverse edges. Mutating the forward edge will automatically update the reverse edge.

**Using Reverse Edges correctly**

In RDF the arrangement of the triples already defines what can be reversed.

```RDF
_:MyObject <myEdge> _:BlankNode  . #That's the right syntax of a reverse edge.
_:BlankNode <dgraph.type> "Person" .
```

The easiest way to correct and apply reverse edges is using JSON. It is simply having the directive on schema at the desired edge. When building your mutations remember that there is no reverse syntax in JSON. So what you should do is similar to RDF: change the arrangement of the JSON objects.

Since `MyObject` is above the `Person` entity, `MyObject` must come before when formatting the mutation.

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

In RDF the correct way to apply reverse edges is very straight-forward.

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

```rdf
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

```graphql
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

```graphql
{
  q(func: has(name)) @filter(eq(name, "Earl Sneed Sinclair")){
    name
    Children : ~parent @facets {
      name
    }
  }
}
```

### Reverse Edges and Facets

Facets on reverse edges are the same as the forward edge. That is, if you set or update a facet on an edge, its reverse will have the same facets.

```rdf
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

```graphql
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

```json
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
 