+++
date = "2017-03-20T22:25:17+11:00"
title = "Blank Nodes and UID"
weight = 2
[menu.main]
    parent = "mutations"
+++

Blank nodes in mutations, written `_:identifier`, identify nodes within a mutation.  Dgraph creates a UID identifying each blank node and returns the created UIDs as the mutation result.  For example, mutation:

```
{
 set {
    _:class <student> _:x .
    _:class <student> _:y .
    _:class <name> "awesome class" .
    _:class <dgraph.type> "Class" .
    _:x <name> "Alice" .
    _:x <dgraph.type> "Person" .
    _:x <dgraph.type> "Student" .
    _:x <planet> "Mars" .
    _:x <friend> _:y .
    _:y <name> "Bob" .
    _:y <dgraph.type> "Person" .
    _:y <dgraph.type> "Student" .
 }
}
```
results in output (the actual UIDs will be different on any run of this mutation)
```
{
  "data": {
    "code": "Success",
    "message": "Done",
    "uids": {
      "class": "0x2712",
      "x": "0x2713",
      "y": "0x2714"
    }
  }
}
```
The graph has thus been updated as if it had stored the triples
```
<0x6bc818dc89e78754> <student> <0xc3bcc578868b719d> .
<0x6bc818dc89e78754> <student> <0xb294fb8464357b0a> .
<0x6bc818dc89e78754> <name> "awesome class" .
<0x6bc818dc89e78754> <dgraph.type> "Class" .
<0xc3bcc578868b719d> <name> "Alice" .
<0xc3bcc578868b719d> <dgraph.type> "Person" .
<0xc3bcc578868b719d> <dgraph.type> "Student" .
<0xc3bcc578868b719d> <planet> "Mars" .
<0xc3bcc578868b719d> <friend> <0xb294fb8464357b0a> .
<0xb294fb8464357b0a> <name> "Bob" .
<0xb294fb8464357b0a> <dgraph.type> "Person" .
<0xb294fb8464357b0a> <dgraph.type> "Student" .
```
The blank node labels `_:class`, `_:x` and `_:y` do not identify the nodes after the mutation, and can be safely reused to identify new nodes in later mutations.

A later mutation can update the data for existing UIDs.  For example, the following to add a new student to the class.
```
{
 set {
    <0x6bc818dc89e78754> <student> _:x .
    _:x <name> "Chris" .
    _:x <dgraph.type> "Person" .
    _:x <dgraph.type> "Student" .
 }
}
```

A query can also directly use UID.
```
{
 class(func: uid(0x6bc818dc89e78754)) {
  name
  student {
   name
   planet
   friend {
    name
   }
  }
 }
}
```