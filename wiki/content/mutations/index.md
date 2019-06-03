+++
title = "Mutations"
+++

Adding or removing data in Dgraph is called a mutation.

A mutation that adds triples is done with the `set` keyword.
```
{
  set {
    # triples in here
  }
}
```

## Triples

The input language is triples in the W3C standard [RDF N-Quad format](https://www.w3.org/TR/n-quads/).

Each triple has the form
```
<subject> <predicate> <object> .
```
Meaning that the graph node identified by `subject` is linked to `object` with directed edge `predicate`.  Each triple ends with a period.  The subject of a triple is always a node in the graph, while the object may be a node or a value (a literal).

For example, the triple
```
<0x01> <name> "Alice" .
```
Represents that graph node with ID `0x01` has a `name` with string value `"Alice"`.  While triple
```
<0x01> <friend> <0x02> .
```
Represents that graph node with ID `0x01` is linked with the `friend` edge to node `0x02`.

Dgraph creates a unique 64 bit identifier for every blank node in the mutation - the node's UID.  A mutation can include a blank node as an identifier for the subject or object, or a known UID from a previous mutation.


## Blank Nodes and UID

Blank nodes in mutations, written `_:identifier`, identify nodes within a mutation.  Dgraph creates a UID identifying each blank node and returns the created UIDs as the mutation result.  For example, mutation:

```
{
 set {
    _:class <student> _:x .
    _:class <student> _:y .
    _:class <name> "awesome class" .
    _:x <name> "Alice" .
    _:x <planet> "Mars" .
    _:x <friend> _:y .
    _:y <name> "Bob" .
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
<0xc3bcc578868b719d> <name> "Alice" .
<0xc3bcc578868b719d> <planet> "Mars" .
<0xc3bcc578868b719d> <friend> <0xb294fb8464357b0a> .
<0xb294fb8464357b0a> <name> "Bob" .
```
The blank node labels `_:class`, `_:x` and `_:y` do not identify the nodes after the mutation, and can be safely reused to identify new nodes in later mutations.

A later mutation can update the data for existing UIDs.  For example, the following to add a new student to the class.
```
{
 set {
    <0x6bc818dc89e78754> <student> _:x .
    _:x <name> "Chris" .
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

## External IDs

Dgraph's input language, RDF, also supports triples of the form `<a_fixed_identifier> <predicate> literal/node` and variants on this, where the label `a_fixed_identifier` is intended as a unique identifier for a node.  For example, mixing [schema.org](http://schema.org) identifiers, [the movie database](https://www.themoviedb.org/) identifiers and blank nodes:

```
_:userA <http://schema.org/type> <http://schema.org/Person> .
_:userA <http://schema.org/name> "FirstName LastName" .
<https://www.themoviedb.org/person/32-robin-wright> <http://schema.org/type> <http://schema.org/Person> .
<https://www.themoviedb.org/person/32-robin-wright> <http://schema.org/name> "Robin Wright" .
```

As of version 0.8 Dgraph doesn't natively support such external IDs as node identifiers.  Instead, external IDs can be stored as properties of a node with an `xid` edge.  For example, from the above, the predicate names are valid in Dgraph, but the node identified with `<http://schema.org/Person>` could be identified in Dgraph with a UID, say `0x123`, and an edge

```
<0x123> <xid> "http://schema.org/Person" .
```

While Robin Wright might get UID `0x321` and triples

```
<0x321> <xid> "https://www.themoviedb.org/person/32-robin-wright" .
<0x321> <http://schema.org/type> <0x123> .
<0x321> <http://schema.org/name> "Robin Wright" .
```

An appropriate schema might be as follows.
```
xid: string @index(exact) .
<http://schema.org/type>: [uid] @reverse .
```

Query Example: All people.

```
{
  var(func: eq(xid, "http://schema.org/Person")) {
    allPeople as <~http://schema.org/type>
  }

  q(func: uid(allPeople)) {
    <http://schema.org/name>
  }
}
```

Query Example: Robin Wright by external ID.

```
{
  robin(func: eq(xid, "https://www.themoviedb.org/person/32-robin-wright")) {
    expand(_all_) { expand(_all_) }
  }
}

```

{{% notice "note" %}} `xid` edges are not added automatically in mutations.  In general it is a user's responsibility to check for existing `xid`'s and add nodes and `xid` edges if necessary. Dgraph leaves all checking of uniqueness of such `xid`'s to external processes. {{% /notice %}}



## Language and RDF Types

RDF N-Quad allows specifying a language for string values and an RDF type.  Languages are written using `@lang`. For example
```
<0x01> <name> "Adelaide"@en .
<0x01> <name> "Аделаида"@ru .
<0x01> <name> "Adélaïde"@fr .
```
See also [how language strings are handled in queries]({{< relref "query-language/index.md#language-support" >}}).

RDF types are attached to literals with the standard `^^` separator.  For example
```
<0x01> <age> "32"^^<xs:int> .
<0x01> <birthdate> "1985-06-08"^^<xs:dateTime> .
```

The supported [RDF datatypes](https://www.w3.org/TR/rdf11-concepts/#section-Datatypes) and the corresponding internal type in which the data is stored are as follows.

| Storage Type                                            | Dgraph type    |
| -------------                                           | :------------: |
| &#60;xs:string&#62;                                     | `string`         |
| &#60;xs:dateTime&#62;                                   | `dateTime`       |
| &#60;xs:date&#62;                                       | `datetime`       |
| &#60;xs:int&#62;                                        | `int`            |
| &#60;xs:boolean&#62;                                    | `bool`           |
| &#60;xs:double&#62;                                     | `float`          |
| &#60;xs:float&#62;                                      | `float`          |
| &#60;geo:geojson&#62;                                   | `geo`            |
| &#60;xs:password&#62;                                   | `password`       |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#string&#62;   | `string`         |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#dateTime&#62; | `dateTime`       |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#date&#62;     | `dateTime`       |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#int&#62;      | `int`            |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#boolean&#62;  | `bool`           |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#double&#62;   | `float`          |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#float&#62;    | `float`          |


See the section on [RDF schema types]({{< relref "#rdf-types" >}}) to understand how RDF types affect mutations and storage.


## Batch mutations

Each mutation may contain multiple RDF triples. For large data uploads many such mutations can be batched in parallel.  The command `dgraph live` does just this; by default batching 1000 RDF lines into a query, while running 100 such queries in parallel.

`dgraph live` takes as input gzipped N-Quad files (that is triple lists without `{ set {`) and batches mutations for all triples in the input.  The tool has documentation of options.

```
dgraph live --help
```
See also [Bulk Data Loading](/deploy#bulk-data-loading).

## Delete

A delete mutation, signified with the `delete` keyword, removes triples from the store.

For example, if the store contained
```
<0xf11168064b01135b> <name> "Lewis Carrol"
<0xf11168064b01135b> <died> "1998"
```

Then delete mutation

```
{
  delete {
     <0xf11168064b01135b> <died> "1998" .
  }
}
```

Deletes the erroneous data and removes it from indexes if present.

For a particular node `N`, all data for predicate `P` (and corresponding indexing) is removed with the pattern `S P *`.

```
{
  delete {
     <0xf11168064b01135b> <author.of> * .
  }
}
```

The pattern `S * *` deletes all known edges out of a node (the node itself may
remain as the target of edges), any reverse edges corresponding to the removed
edges and any indexing for the removed data. The predicates to delete are
derived from the type information for that node (the value of the `dgraph.type`
edges on that node and their corresponding definitions in the schema). If that
information is missing, this operation will be a no-op.

```
{
  delete {
     <0xf11168064b01135b> * * .
  }
}
```


{{% notice "note" %}} The patterns `* P O` and `* * O` are not supported since its expensive to store/find all the incoming edges. {{% /notice %}}

### Deletion of non-list predicates

Deleting the value of a non-list predicate (i.e a 1-to-1 relation) can be done in two ways.

1. Using the star notation mentioned in the last section.
1. Setting the object to a specific value. If the value passed is not the current value, the mutation will succeed but will have no effect. If the value passed is the current value, the mutation will succeed and will delete the triple.

For language-tagged values, the following special syntax is supported:

```
{
	delete {
		<0x12345> <name@es> * .
	}
}
```

In this example, the value of name tagged with language tag `es` will be deleted.
Other tagged values are left untouched.

## Mutations using cURL

Mutations can be done over HTTP by making a `POST` request to an Alpha's `/mutate` endpoint. On the command line this can be done with curl. To commit the mutation, pass the parameter `commitNow=true` in the URL.

To run a `set` mutation:

```sh
curl -H "Content-Type: application/rdf" -X POST localhost:8080/mutate?commitNow=true -d $'
{
  set {
    _:alice <name> "Alice" .
  }
}'
```

To run a `delete` mutation:

```sh
curl -H "Content-Type: application/rdf" -X POST localhost:8080/mutate?commitNow=true -d $'
{
  delete {
    # Example: Alice's UID is 0x56f33
    <0x56f33> <name> * .
  }
}'
```

To run an RDF mutation stored in a file, use curl's `--data-binary` option so that, unlike the `-d` option, the data is not URL encoded.

```sh
curl -H "Content-Type: application/rdf" -X POST localhost:8080/mutate?commitNow=true --data-binary @mutation.txt
```

## JSON Mutation Format

Mutations can also be specified using JSON objects. This can allow mutations to
be expressed in a more natural way. It also eliminates the need for apps to
have custom serialisation code, since most languages already have a JSON
marshalling library.

When Dgraph receives a mutation as a JSON object, it first converts in into
multiple RDFs that are then processed as normal.

Each JSON object represents a single node in the graph.

{{% notice "note" %}}
JSON mutations are available via gRPC clients such as the Go client, JS client, and Java client, and are available to HTTP clients with [dgraph-js-http](https://github.com/dgraph-io/dgraph-js-http) and cURL. See more about cURL [here]({{< relref "#using-json-operations-via-curl" >}})
{{% /notice %}}

### Setting literal values

When setting new values, the `set_json` field in the `Mutation` message should
contain a JSON object.

Literal values can be set by adding a key/value to the JSON object. The key
represents the predicate, and the value represents the object.

For example:
```json
{
  "name": "diggy",
  "food": "pizza"
}
```
Will be converted into the RDFs:
```
_:blank-0 <name> "diggy" .
_:blank-0 <food> "pizza" .
```

The result of the mutation would also contain a map, which would have the uid assigned corresponding
to the key `blank-0`. You could specify your own key like

```json
{
  "uid": "_:diggy",
  "name": "diggy",
  "food": "pizza"
}
```

In this case, the assigned uids map would have a key called `diggy` with the value being the uid
assigned to it.

### Language support

An important difference between RDF and JSON mutations is in regards to specifying a string value's
language. In JSON, the language tag is appended to the edge _name_, not the value like in RDF.

For example, the JSON mutation
```json
{
  "food": "taco",
  "rating@en": "tastes good",
  "rating@es": "sabe bien",
  "rating@fr": "c'est bon",
  "rating@it": "è buono"
}
```

is equivalent to the following RDF:
```
_:blank-0 <food> "taco" .
_:blank-0 <rating> "tastes good"@en .
_:blank-0 <rating> "sabe bien"@es .
_:blank-0 <rating> "c'est bon"@fr .
_:blank-0 <rating> "è buono"@it .
```

### Referencing existing nodes

If a JSON object contains a field named `"uid"`, then that field is interpreted
as the UID of an existing node in the graph. This mechanism allows you to
reference existing nodes.

For example:
```json
{
  "uid": "0x467ba0",
  "food": "taco",
  "rating": "tastes good",
}
```
Will be converted into the RDFs:
```
<0x467ba0> <food> "taco" .
<0x467ba0> <rating> "tastes good" .
```

### Edges between nodes

Edges between nodes are represented in a similar way to literal values, except
that the object is a JSON object.

For example:
```json
{
  "name": "Alice",
  "friend": {
    "name": "Betty"
  }
}
```
Will be converted into the RDFs:
```
_:blank-0 <name> "Alice" .
_:blank-0 <friend> _:blank-1 .
_:blank-1 <name> "Betty" .
```

The result of the mutation would contain the uids assigned to `blank-0` and `blank-1` nodes. If you
wanted to return these uids under a different key, you could specify the `uid` field as a blank
node.

```json
{
  "uid": "_:alice",
  "name": "Alice",
  "friend": {
    "uid": "_:bob",
    "name": "Betty"
  }
}
```
Will be converted to:
```
_:alice <name> "Alice" .
_:alice <friend> _:bob .
_:bob <name> "Betty" .
```

Existing nodes can be referenced in the same way as when adding literal values.
E.g. to link two existing nodes:
```json
{
  "uid": "0x123",
  "link": {
    "uid": "0x456"
  }
}
```

Will be converted to:

```JSON
<0x123> <link> <0x456> .
```

{{% notice "note" %}}
A common mistake is to attempt to use `{"uid":"0x123","link":"0x456"}`.  This
will result in an error. Dgraph interprets this JSON object as setting the
`link` predicate to the string`"0x456"`, which is usually not intended.  {{%
/notice %}}

### Deleting literal values

Deletion mutations can also be sent in JSON format. To send a delete mutation,
use the `delete_json` field instead of the `set_json` field in the `Mutation`
message.

{{% notice "note" %}} Check the [JSON Syntax using Raw HTTP or Ratel UI]({{< relref "#json-syntax-using-raw-http-or-ratel-ui">}}) section if you're using the dgraph-js-http client or Ratel UI. {{% /notice %}}

When using delete mutations, an existing node always has to be referenced. So
the `"uid"` field for each JSON object must be present. Predicates that should
be deleted should be set to the JSON value `null`.

For example, to remove a food rating:
```json
{
  "uid": "0x467ba0",
  "rating": null
}
```

### Deleting edges

Deleting a single edge requires the same JSON object that would create that
edge. E.g. to delete the predicate `link` from `"0x123"` to `"0x456"`:
```json
{
  "uid": "0x123",
  "link": {
    "uid": "0x456"
  }
}
```

All edges for a predicate emanating from a single node can be deleted at once
(corresponding to deleting `S P *`):
```json
{
  "uid": "0x123",
  "link": null
}
```

If no predicates specified, then all of the node's known outbound edges are
deleted (corresponding to deleting `S * *`). The predicates to delete are
derived using the type system. Refer to the [RDF format]({{< relref "#delete" >}})
documentation and the section on the [type system]({{< relref "query-language/index.md#type-system" >}})
for more information:

```json
{
  "uid": "0x123"
}
```

### Facets

Facets can be created by using the `|` character to separate the predicate
and facet key in a JSON object field name. This is the same encoding schema
used to show facets in query results. E.g.
```json
{
  "name": "Carol",
  "name|initial": "C",
  "friend": {
    "name": "Daryl",
    "friend|close": "yes"
  }
}
```
Produces the following RDFs:
```
_:blank-0 <name> "Carol" (initial=C) .
_:blank-0 <friend> _:blank-1 (close=yes) .
_:blank-1 <name> "Daryl" .
```

### Creating a list with JSON and interacting with

Schema:

```JSON
testList: [string] .
```

```JSON
{
  "testList": [
    "Grape",
    "Apple",
    "Strawberry",
    "Banana",
    "watermelon"
  ]
}
```

Let’s then remove "Apple" from this list (Remember, it’s case sensitive):

```JSON
{
   "uid": "0xd", #UID of the list.
   "testList": "Apple"
}
```

{{% notice "note" %}} Check the [JSON Syntax using Raw HTTP or Ratel UI]({{< relref "#json-syntax-using-raw-http-or-ratel-ui">}}) section if you're using the dgraph-js-http client or Ratel UI. {{% /notice %}}


Add another fruit:

```JSON
{
   "uid": "0xd", #UID of the list.
   "testList": "Pineapple"
}
```

### Specifying multiple operations

When specifying add or delete mutations, multiple nodes can be specified
at the same time using JSON arrays.

For example, the following JSON object can be used to add two new nodes, each
with a `name`:

```JSON
[
  {
    "name": "Edward"
  },
  {
    "name": "Fredric"
  }
]
```

### JSON Syntax using Raw HTTP or Ratel UI

This syntax can be used in the most current version of Ratel, in the [dgraph-js-http](https://github.com/dgraph-io/dgraph-js-http) client or even via cURL.

You can also [download the Ratel UI for Linux, macOS, or Windows](https://discuss.dgraph.io/t/ratel-installer-for-linux-macos-and-windows-preview-version-ratel-update-from-v1-0-6/2884/).

Mutate:
```JSON
{
  "set": [
    {
      # One JSON obj in here
    },
    {
      # Another JSON obj in here for multiple operations
    }
  ]
}
```

Delete:

Deletion operations are the same as [Deleting literal values]({{< relref "#deleting-literal-values">}}) and [Deleting edges]({{< relref "#deleting-edges">}}).

```JSON
{
  "delete": [
    {
      # One JSON obj in here
    },
    {
      # Another JSON obj in here for multiple operations
    }
  ]
}
```

### Using JSON operations via cURL

First you have to configure the HTTP header to specify content-type.

```sh
-H 'Content-Type: application/json'
```

{{% notice "note" %}}
In order to use `jq` for JSON formatting you need the `jq` package. See the
[`jq` downloads](https://stedolan.github.io/jq/download/) page for installation
details. You can also use Python's built in `json.tool` module with `python -m
json.tool` to do JSON formatting.
{{% /notice %}}

```sh
curl -H "Content-Type: application/json" -X POST localhost:8080/mutate?commitNow=true -d  $'
    {
      "set": [
        {
          "name": "Alice"
        },
        {
          "name": "Bob"
        }
      ]
    }' | jq

```

To delete:

```sh
curl -H "Content-Type: application/json" -X POST localhost:8080/mutate?commitNow=true -d  $'
    {
      "delete": [
        {
          "uid": "0xa"
        }
      ]
    }' | jq
```

Mutation with a JSON file:

```sh
curl -H "Content-Type: application/json" -X POST localhost:8080/mutate?commitNow=true -d @data.json
```
