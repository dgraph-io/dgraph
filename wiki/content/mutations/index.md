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
<0x01> <dgraph.type> "Person" .
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

## External IDs

Dgraph's input language, RDF, also supports triples of the form `<a_fixed_identifier> <predicate> literal/node` and variants on this, where the label `a_fixed_identifier` is intended as a unique identifier for a node.  For example, mixing [schema.org](http://schema.org) identifiers, [the movie database](https://www.themoviedb.org/) identifiers and blank nodes:

```
_:userA <http://schema.org/type> <http://schema.org/Person> .
_:userA <dgraph.type> "Person" .
_:userA <http://schema.org/name> "FirstName LastName" .
<https://www.themoviedb.org/person/32-robin-wright> <http://schema.org/type> <http://schema.org/Person> .
<https://www.themoviedb.org/person/32-robin-wright> <http://schema.org/name> "Robin Wright" .
```

As Dgraph doesn't natively support such external IDs as node identifiers.  Instead, external IDs can be stored as properties of a node with an `xid` edge.  For example, from the above, the predicate names are valid in Dgraph, but the node identified with `<http://schema.org/Person>` could be identified in Dgraph with a UID, say `0x123`, and an edge

```
<0x123> <xid> "http://schema.org/Person" .
<0x123> <dgraph.type> "ExternalType" .
```

While Robin Wright might get UID `0x321` and triples

```
<0x321> <xid> "https://www.themoviedb.org/person/32-robin-wright" .
<0x321> <http://schema.org/type> <0x123> .
<0x321> <http://schema.org/name> "Robin Wright" .
<0x321> <dgraph.type> "Person" .
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

## External IDs and Upsert Block

The upsert block makes managing external IDs easy.

Set the schema.
```
xid: string @index(exact) .
<http://schema.org/name>: string @index(exact) .
<http://schema.org/type>: [uid] @reverse .
```

Set the type first of all.
```
{
  set {
    _:blank <xid> "http://schema.org/Person" .
    _:blank <dgraph.type> "ExternalType" .
  }
}
```

Now you can create a new person and attach its type using the upsert block.
```
   upsert {
      query {
        var(func: eq(xid, "http://schema.org/Person")) {
          Type as uid
        }
        var(func: eq(<http://schema.org/name>, "Robin Wright")) {
          Person as uid
        }
      }
      mutation {
          set {
           uid(Person) <xid> "https://www.themoviedb.org/person/32-robin-wright" .
           uid(Person) <http://schema.org/type> uid(Type) .
           uid(Person) <http://schema.org/name> "Robin Wright" .
           uid(Person) <dgraph.type> "Person" .
          }
      }
    }
```

You can also delete a person and detach the relation between Type and Person Node. It's the same as above, but you use the keyword "delete" instead of "set". "`http://schema.org/Person`" will remain but "`Robin Wright`" will be deleted.

```
   upsert {
      query {
        var(func: eq(xid, "http://schema.org/Person")) {
          Type as uid
        }
        var(func: eq(<http://schema.org/name>, "Robin Wright")) {
          Person as uid
        }
      }
      mutation {
          delete {
           uid(Person) <xid> "https://www.themoviedb.org/person/32-robin-wright" .
           uid(Person) <http://schema.org/type> uid(Type) .
           uid(Person) <http://schema.org/name> "Robin Wright" .
           uid(Person) <dgraph.type> "Person" .
          }
      }
    }
```

Query by user.
```
{
  q(func: eq(<http://schema.org/name>, "Robin Wright")) {
    uid
    xid
    <http://schema.org/name>
    <http://schema.org/type> {
      uid
      xid
    }
  }
}
```

## Language and RDF Types

RDF N-Quad allows specifying a language for string values and an RDF type.  Languages are written using `@lang`. For example
```
<0x01> <name> "Adelaide"@en .
<0x01> <name> "Аделаида"@ru .
<0x01> <name> "Adélaïde"@fr .
<0x01> <dgraph.type> "Person" .
```
See also [how language strings are handled in queries]({{< relref "query-language/index.md#language-support" >}}).

RDF types are attached to literals with the standard `^^` separator.  For example
```
<0x01> <age> "32"^^<xs:int> .
<0x01> <birthdate> "1985-06-08"^^<xs:dateTime> .
```

The supported [RDF datatypes](https://www.w3.org/TR/rdf11-concepts/#section-Datatypes) and the corresponding internal type in which the data is stored are as follows.

| Storage Type                                                    | Dgraph type     |
| -------------                                                   | :------------:   |
| &#60;xs:string&#62;                                             | `string`         |
| &#60;xs:dateTime&#62;                                           | `dateTime`       |
| &#60;xs:date&#62;                                               | `datetime`       |
| &#60;xs:int&#62;                                                | `int`            |
| &#60;xs:integer&#62;                                            | `int`            |
| &#60;xs:boolean&#62;                                            | `bool`           |
| &#60;xs:double&#62;                                             | `float`          |
| &#60;xs:float&#62;                                              | `float`          |
| &#60;geo:geojson&#62;                                           | `geo`            |
| &#60;xs:password&#62;                                           | `password`       |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#string&#62;           | `string`         |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#dateTime&#62;         | `dateTime`       |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#date&#62;             | `dateTime`       |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#int&#62;              | `int`            |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#positiveInteger&#62;  | `int`            |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#integer&#62;          | `int`            |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#boolean&#62;          | `bool`           |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#double&#62;           | `float`          |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#float&#62;            | `float`          |


See the section on [RDF schema types]({{< relref "#rdf-types" >}}) to understand how RDF types affect mutations and storage.


## Batch mutations

Each mutation may contain multiple RDF triples. For large data uploads many such mutations can be batched in parallel.  The command `dgraph live` does just this; by default batching 1000 RDF lines into a query, while running 100 such queries in parallel.

`dgraph live` takes as input gzipped N-Quad files (that is triple lists without `{ set {`) and batches mutations for all triples in the input.  The tool has documentation of options.

```
dgraph live --help
```
See also [Fast Data Loading](/deploy#fast-data-loading).

## Delete

A delete mutation, signified with the `delete` keyword, removes triples from the store.

For example, if the store contained
```
<0xf11168064b01135b> <name> "Lewis Carrol"
<0xf11168064b01135b> <died> "1998"
<0xf11168064b01135b> <dgraph.type> "Person" .
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
    _:alice <dgraph.type> "Person" .
  }
}'
```

To run a `delete` mutation:

```sh
curl -H "Content-Type: application/rdf" -X POST localhost:8080/mutate?commitNow=true -d $'
{
  delete {
    # Example: The UID of Alice is 0x56f33
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
  "food": "pizza",
  "dgraph.type": "Mascot"
}
```
Will be converted into the RDFs:
```
_:blank-0 <name> "diggy" .
_:blank-0 <food> "pizza" .
_:blank-0 <dgraph.type> "Mascot" .
```

The result of the mutation would also contain a map, which would have the uid assigned corresponding
to the key `blank-0`. You could specify your own key like

```json
{
  "uid": "_:diggy",
  "name": "diggy",
  "food": "pizza",
  "dgraph.type": "Mascot"
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
  "rating@it": "è buono",
  "dgraph.type": "Food"
}
```

is equivalent to the following RDF:
```
_:blank-0 <food> "taco" .
_:blank-0 <dgraph.type> "Food" .
_:blank-0 <rating> "tastes good"@en .
_:blank-0 <rating> "sabe bien"@es .
_:blank-0 <rating> "c'est bon"@fr .
_:blank-0 <rating> "è buono"@it .
```

### Geolocation support

Support for geolocation data is available in JSON. Geo-location data is entered
as a JSON object with keys "type" and "coordinates". Keep in mind we only
support indexing on the Point, Polygon, and MultiPolygon types, but we can store
other types of geolocation data. Below is an example:

```
{
  "food": "taco",
  "location": {
    "type": "Point",
    "coordinates": [1.0, 2.0]
  }
}
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
  "dgraph.type": "Food"
}
```
Will be converted into the RDFs:
```
<0x467ba0> <food> "taco" .
<0x467ba0> <rating> "tastes good" .
<0x467ba0> <dgraph.type> "Food" .
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

If no predicates are specified, then all of the node's known outbound edges (to
other nodes and to literal values) are deleted (corresponding to deleting `S *
*`). The predicates to delete are derived using the type system. Refer to the
[RDF format]({{< relref "#delete" >}}) documentation and the section on the
[type system]({{< relref "query-language/index.md#type-system" >}}) for more
information:

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
  "dgraph.type": "Person",
  "friend": {
    "name": "Daryl",
    "friend|close": "yes",
    "dgraph.type": "Person"
  }
}
```
Produces the following RDFs:
```
_:blank-0 <name> "Carol" (initial=C) .
_:blank-0 <dgraph.type> "Person" .
_:blank-0 <friend> _:blank-1 (close=yes) .
_:blank-1 <name> "Daryl" .
_:blank-1 <dgraph.type> "Person" .
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
curl -H "Content-Type: application/json" -X POST localhost:8080/mutate?commitNow=true -d $'
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
curl -H "Content-Type: application/json" -X POST localhost:8080/mutate?commitNow=true -d $'
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

where the contents of data.json looks like the following:

```json
{
  "set": [
    {
      "name": "Alice"
    },
    {
      "name": "Bob"
    }
  ]
}
```

The JSON file must follow the same format for mutations over HTTP: a single JSON
object with the `"set"` or `"delete"` key and an array of JSON objects for the
mutation. If you already have a file with an array of data, you can use `jq` to
transform your data to the proper format. For example, if your data.json file
looks like this:

```json
[
  {
    "name": "Alice"
  },
  {
    "name": "Bob"
  }
]
```

then you can transform your data to the proper format with the following `jq`
command, where the `.` in the `jq` string represents the contents of data.json:

```sh
cat data.json | jq '{set: .}'
```

```
{
  "set": [
    {
      "name": "Alice"
    },
    {
      "name": "Bob"
    }
  ]
}
```

## Upsert Block

The upsert block allows performing queries and mutations in a single request. The upsert
block contains one query block and one or more than one mutation blocks. Variables defined
in the query block can be used in the mutation blocks using the `uid` and `val` function.

In general, the structure of the upsert block is as follows:

```
upsert {
  query <query block>
  [fragment <fragment block>]
  mutation <mutation block 1>
  [mutation <mutation block 2>]
  ...
}
```

Execution of an upsert block also returns the response of the query executed on the state
of the database *before mutation was executed*. To get the latest result, we should commit
the mutation and execute another query.

### `uid` Function

The `uid` function allows extracting UIDs from variables defined in the query block.
There are two possible outcomes based on the results of executing the query block:

* If the variable is empty i.e. no node matched the query, the `uid` function returns a new UID in case of a `set` operation and is thus treated similar to a blank node. On the other hand, for `delete/del` operation, it returns no UID, and thus the operation becomes a no-op and is silently ignored. A blank node gets the same UID across all the mutation blocks.
* If the variable stores one or more than one UIDs, the `uid` function returns all the UIDs stored in the variable. In this case, the operation is performed on all the UIDs returned, one at a time.

### `val` Function

The `val` function allows extracting values from value variables. Value variables store
a mapping from UIDs to their corresponding values. Hence, `val(v)` is replaced by the value
stored in the mapping for the UID (Subject) in the N-Quad. If the variable `v` has no value
for a given UID, the mutation is silently ignored. The `val` function can be used with the
result of aggregate variables as well, in which case, all the UIDs in the mutation would
be updated with the aggregate value.

### Example of `uid` Function

Consider an example with the following schema:

```sh
curl localhost:8080/alter -X POST -d $'
  name: string @index(term) .
  email: string @index(exact, trigram) @upsert .
  age: int @index(int) .' | jq
```

Now, let's say we want to create a new user with `email` and `name` information.
We also want to make sure that one email has exactly one corresponding user in
the database. To achieve this, we need to first query whether a user exists
in the database with the given email. If a user exists, we use its UID
to update the `name` information. If the user doesn't exist, we create
a new user and update the `email` and `name` information.

We can do this using the upsert block as follows:

```sh
curl -H "Content-Type: application/rdf" -X POST localhost:8080/mutate?commitNow=true -d $'
upsert {
  query {
    q(func: eq(email, "user@company1.io")) {
      v as uid
      name
    }
  }

  mutation {
    set {
      uid(v) <name> "first last" .
      uid(v) <email> "user@company1.io" .
    }
  }
}' | jq
```

Result:

```json
{
  "data": {
    "q": [],
    "code": "Success",
    "message": "Done",
    "uids": {
      "uid(v)": "0x1"
    }
  },
  "extensions": {...}
}
```

The query part of the upsert block stores the UID of the user with the provided email
in the variable `v`. The mutation part then extracts the UID from variable `v`, and
stores the `name` and `email` information in the database. If the user exists,
the information is updated. If the user doesn't exist, `uid(v)` is treated
as a blank node and a new user is created as explained above.

If we run the same mutation again, the data would just be overwritten, and no new uid is
created. Note that the `uids` map is empty in the result when the mutation is executed
again and the `data` map (key `q`) contains the uid that was created in the previous upsert.

```json
{
  "data": {
    "q": [
      {
        "uid": "0x1",
        "name": "first last"
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
curl -H "Content-Type: application/json" -X POST localhost:8080/mutate?commitNow=true -d '
{
  "query": "{ q(func: eq(email, \\"user@company1.io\\")) {v as uid\\n name} }",
  "set": {
    "uid": "uid(v)",
    "name": "first last",
    "email": "user@company1.io"
  }
}' | jq
```

Now, we want to add the `age` information for the same user having the same email
`user@company1.io`. We can use the upsert block to do the same as follows:

```sh
curl -H "Content-Type: application/rdf" -X POST localhost:8080/mutate?commitNow=true -d $'
upsert {
  query {
    q(func: eq(email, "user@company1.io")) {
      v as uid
    }
  }

  mutation {
    set {
      uid(v) <age> "28" .
    }
  }
}' | jq
```

Result:

```json
{
  "data": {
    "q": [
      {
        "uid": "0x1"
      }
    ],
    "code": "Success",
    "message": "Done",
    "uids": {}
  },
  "extensions": {...}
}
```

Here, the query block queries for a user with `email` as `user@company1.io`. It stores
the `uid` of the user in variable `v`. The mutation block then updates the `age` of the
user by extracting the uid from the variable `v` using `uid` function.

We can achieve the same result using `json` dataset as follows:

```sh
curl -H "Content-Type: application/json" -X POST localhost:8080/mutate?commitNow=true -d $'
{
  "query": "{ q(func: eq(email, \\"user@company1.io\\")) {v as uid} }",
  "set":{
    "uid": "uid(v)",
    "age": "28"
  }
}' | jq
```

If we want to execute the mutation only when the user exists, we could use
[Conditional Upsert]({{< relref "#conditional-upsert" >}}).

### Example of `val` Function

Let's say we want to migrate the predicate `age` to `other`. We can do this using the
following mutation:

```sh
curl -H "Content-Type: application/rdf" -X POST localhost:8080/mutate?commitNow=true -d $'
upsert {
  query {
    v as var(func: has(age)) {
      a as age
    }
  }

  mutation {
    # we copy the values from the old predicate
    set {
      uid(v) <other> val(a) .
    }

    # and we delete the old predicate
    delete {
      uid(v) <age> * .
    }
  }
}' | jq
```

Result:

```json
{
  "data": {
    "code": "Success",
    "message": "Done",
    "uids": {}
  },
  "extensions": {...}
}
```

Here, variable `a` will store a mapping from all the UIDs to their `age`. The mutation
block then stores the corresponding value of `age` for each UID in the `other` predicate
and deletes the `age` predicate.

We can achieve the same result using `json` dataset as follows:

```sh
curl -H "Content-Type: application/json" -X POST localhost:8080/mutate?commitNow=true -d $'{
  "query": "{ v as var(func: regexp(email, /.*@company1.io$/)) }",
  "delete": {
    "uid": "uid(v)",
    "age": null
  },
  "set": {
    "uid": "uid(v)",
    "other": "val(a)"
  }
}' | jq
```

### Bulk Delete Example

Let's say we want to delete all the users of `company1` from the database. This can be
achieved in just one query using the upsert block as follows:

```sh
curl -H "Content-Type: application/rdf" -X POST localhost:8080/mutate?commitNow=true -d $'
upsert {
  query {
    v as var(func: regexp(email, /.*@company1.io$/))
  }

  mutation {
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
  "delete": {
    "uid": "uid(v)",
    "name": null,
    "email": null,
    "age": null
  }
}' | jq
```

## Conditional Upsert

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

### Example of Conditional Upsert

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

### Example of Multiple Mutation Blocks

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
