+++
date = "2017-03-20T22:25:17+11:00"
title = "Triples"
weight = 1
[menu.main]
    parent = "mutations"
+++

A mutation that adds triples is done with the `set` keyword.
```
{
  set {
    # triples in here
  }
}
```

The input language is triples in the W3C standard [RDF N-Quad format](https://www.w3.org/TR/n-quads/).

Each triple has the form
```
<subject> <predicate> <object> .
```
Meaning that the graph node identified by `subject` is linked to `object` with directed edge `predicate`.  Each triple ends with a period.  The subject of a triple is always a node in the graph, while the object may be a node or a value (a literal).

For example, the triple
```
<0x01> <name> "Alice" .
<0x01> <dgraph.type> "Person" .
```
Represents that graph node with ID `0x01` has a `name` with string value `"Alice"`.  While triple
```
<0x01> <friend> <0x02> .
```
Represents that graph node with ID `0x01` is linked with the `friend` edge to node `0x02`.

Dgraph creates a unique 64 bit identifier for every blank node in the mutation - the node's UID.  A mutation can include a blank node as an identifier for the subject or object, or a known UID from a previous mutation.