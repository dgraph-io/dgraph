+++
date = "2017-03-20T22:25:17+11:00"
title = "JSON Mutation Format"
weight = 10
[menu.main]
    parent = "mutations"
+++

Mutations can also be specified using JSON objects. This can allow mutations to
be expressed in a more natural way. It also eliminates the need for apps to
have custom serialisation code, since most languages already have a JSON
marshalling library.

When Dgraph receives a mutation as a JSON object, it first converts it into an 
internal edge format that is then processed into Dgraph.

> JSON  -> Edges -> Posting list \
> RDF   -> Edges -> Posting list

Each JSON object represents a single node in the graph.

{{% notice "note" %}}
JSON mutations are available via gRPC clients such as the Go client, JS client, and Java client, and are available to HTTP clients with [dgraph-js-http](https://github.com/dgraph-io/dgraph-js-http) and cURL. See more about cURL [here]({{< relref "#using-json-operations-via-curl" >}})
{{% /notice %}}

## Setting literal values

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
```RDF
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

## Language support

An important difference between RDF and JSON mutations is in regards to specifying a string value's
language. In JSON, the language tag is appended to the edge _name_, not the value like in RDF.

For example, the JSON mutation
```JSON
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
```RDF
_:blank-0 <food> "taco" .
_:blank-0 <dgraph.type> "Food" .
_:blank-0 <rating> "tastes good"@en .
_:blank-0 <rating> "sabe bien"@es .
_:blank-0 <rating> "c'est bon"@fr .
_:blank-0 <rating> "è buono"@it .
```

## Geolocation support

Support for geolocation data is available in JSON. Geo-location data is entered
as a JSON object with keys "type" and "coordinates". Keep in mind we only
support indexing on the Point, Polygon, and MultiPolygon types, but we can store
other types of geolocation data. Below is an example:

```JSON
{
  "food": "taco",
  "location": {
    "type": "Point",
    "coordinates": [1.0, 2.0]
  }
}
```

## Referencing existing nodes

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
```RDF
<0x467ba0> <food> "taco" .
<0x467ba0> <rating> "tastes good" .
<0x467ba0> <dgraph.type> "Food" .
```

## Edges between nodes

Edges between nodes are represented in a similar way to literal values, except
that the object is a JSON object.

For example:
```JSON
{
  "name": "Alice",
  "friend": {
    "name": "Betty"
  }
}
```
Will be converted into the RDFs:
```RDF
_:blank-0 <name> "Alice" .
_:blank-0 <friend> _:blank-1 .
_:blank-1 <name> "Betty" .
```

The result of the mutation would contain the uids assigned to `blank-0` and `blank-1` nodes. If you
wanted to return these uids under a different key, you could specify the `uid` field as a blank
node.

```JSON
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

```RDF
_:alice <name> "Alice" .
_:alice <friend> _:bob .
_:bob <name> "Betty" .
```

Existing nodes can be referenced in the same way as when adding literal values.
E.g. to link two existing nodes:
```JSON
{
  "uid": "0x123",
  "link": {
    "uid": "0x456"
  }
}
```

Will be converted to:

```RDF
<0x123> <link> <0x456> .
```

{{% notice "note" %}}
A common mistake is to attempt to use `{"uid":"0x123","link":"0x456"}`.  This
will result in an error. Dgraph interprets this JSON object as setting the
`link` predicate to the string`"0x456"`, which is usually not intended.  {{%
/notice %}}

## Deleting literal values

Deletion mutations can also be sent in JSON format. To send a delete mutation,
use the `delete_json` field instead of the `set_json` field in the `Mutation`
message.

{{% notice "note" %}} Check the [JSON Syntax using Raw HTTP or Ratel UI]({{< relref "#json-syntax-using-raw-http-or-ratel-ui">}}) section if you're using the dgraph-js-http client or Ratel UI. {{% /notice %}}

When using delete mutations, an existing node always has to be referenced. So
the `"uid"` field for each JSON object must be present. Predicates that should
be deleted should be set to the JSON value `null`.

For example, to remove a food rating:
```JSON
{
  "uid": "0x467ba0",
  "rating": null
}
```

## Deleting edges

Deleting a single edge requires the same JSON object that would create that
edge. E.g. to delete the predicate `link` from `"0x123"` to `"0x456"`:
```JSON
{
  "uid": "0x123",
  "link": {
    "uid": "0x456"
  }
}
```

All edges for a predicate emanating from a single node can be deleted at once
(corresponding to deleting `S P *`):
```JSON
{
  "uid": "0x123",
  "link": null
}
```

If no predicates are specified, then all of the node's known outbound edges (to
other nodes and to literal values) are deleted (corresponding to deleting `S *
*`). The predicates to delete are derived using the type system. Refer to the
[RDF format]({{< relref "mutations/delete.md" >}}) documentation and the section on the
[type system]({{< relref "query-language/type-system.md" >}}) for more
information:

```JSON
{
  "uid": "0x123"
}
```

## Facets

Facets can be created by using the `|` character to separate the predicate
and facet key in a JSON object field name. This is the same encoding schema
used to show facets in query results. E.g.
```JSON
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

Facets do not contain type information but Dgraph will try to guess a type from
the input. If the value of a facet can be parsed to a number, it will be
converted to either a float or an int. If it can be parsed as a boolean, it will
be stored as a boolean. If the value is a string, it will be stored as a
datetime if the string matches one of the time formats that Dgraph recognizes
(YYYY, MM-YYYY, DD-MM-YYYY, RFC339, etc.) and as a double-quoted string
otherwise. If you do not want to risk the chance of your facet data being
misinterpreted as a time value, it is best to store numeric data as either an
int or a float.

## Deleting Facets

The easiest way to delete a Facet is overwriting it. When you create a new mutation for the same entity without a facet, the existing facet will be deleted automatically.

e.g:

```RDF
<0x1> <name> "Carol" .
<0x1> <friend> <0x2> .
```

Another way to do this is by using the Upsert Block.

> In this query below, we are deleting Facet in the Name and Friend predicates. To overwrite we need to collect the values ​​of the edges on which we are performing this operation and use the function "val(var)" to complete the overwriting.

```sh
curl -H "Content-Type: application/rdf" -X POST localhost:8080/mutate?commitNow=true -d $'
upsert {
  query {
    user as var(func: eq(name, "Carol")){
      Name as name
      Friends as friend
    }
 }

  mutation {
    set {
      uid(user) <name> val(Name) .
      uid(user) <friend> uid(Friends) .
    }
  }
}' | jq
```

## Creating a list with JSON and interacting with

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

```graphql
{
  q(func: has(testList)) {
    uid
    testList
  }
}
```

```JSON
{
  "delete": {
    "uid": "0x6", #UID of the list.
    "testList": "Apple"
  }
}
```

Also you can delete multiple values
```JSON
{
  "delete": {
    "uid": "0x6",
    "testList": [
          "Strawberry",
          "Banana",
          "watermelon"
        ]
  }
}
```

{{% notice "note" %}} Check the [JSON Syntax using Raw HTTP or Ratel UI]({{< relref "#json-syntax-using-raw-http-or-ratel-ui">}}) section if you're using the dgraph-js-http client or Ratel UI. {{% /notice %}}

Add another fruit:

```JSON
{
   "uid": "0x6", #UID of the list.
   "testList": "Pineapple"
}
```

## Facets in List-type with JSON
Schema:
```sh
<name>: string @index(exact).
<nickname>: [string] .
```
To create a List-type predicate you need to specify all value in a single list. Facets for all
predicate values should be specified together. It is done in map format with index of predicate
values inside list being map key and their respective facets value as map values. Predicate values
which does not have facets values will be missing from facets map. E.g.
```JSON
{
  "set": [
    {
      "uid": "_:Julian",
      "name": "Julian",
      "nickname": ["Jay-Jay", "Jules", "JB"],
      "nickname|kind": {
        "0": "first",
        "1": "official",
        "2": "CS-GO"
      }
    }
  ]
}
```
Above you see that we have three values ​​to enter the list with their respective facets.
You can run this query to check the list with facets:
```graphql
{
   q(func: eq(name,"Julian")) {
    uid
    nickname @facets
   }
}
```
Later, if you want to add more values ​​with facets, just do the same procedure, but this time instead of using Blank-node you must use the actual node's UID.
```JSON
{
  "set": [
    {
      "uid": "0x3",
      "nickname|kind": "Internet",
      "nickname": "@JJ"
    }
  ]
}
```
And the final result is:
```JSON
{
  "data": {
    "q": [
      {
        "uid": "0x3",
        "nickname|kind": {
          "0": "first",
          "1": "Internet",
          "2": "official",
          "3": "CS-GO"
        },
        "nickname": [
          "Jay-Jay",
          "@JJ",
          "Jules",
          "JB"
        ]
      }
    ]
  }
}
```

## Specifying multiple operations

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

## JSON Syntax using Raw HTTP or Ratel UI

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

## Using JSON operations via cURL

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
