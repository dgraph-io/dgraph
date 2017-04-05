+++
title = "Query Language"
+++

## GraphQL+-
Dgraph uses a variation of [GraphQL](https://facebook.github.io/graphql/) as the primary language of communication.
GraphQL is a query language created by Facebook for describing the capabilities and requirements of data models for client‐server applications.
While GraphQL isn't aimed at Graph databases, it's graph-like query syntax, schema validation and subgraph shaped response make it a great language choice.
Having said that, we have modified GraphQL to support graph operations and removed some of the features that we felt weren't a right fit to be a language for a graph database.
We're calling this simplified, feature rich language, ''GraphQL+-''.

{{% notice "note" %}}This language is a work in progress. We're adding more features and we might further simplify some of the existing ones.{{% /notice %}}

## Mutations

{{% notice "note" %}}All mutations queries shown here can be run using `curl localhost:8080/query -XPOST -d $'mutation { ... }'`{{% /notice %}}

To add data to Dgraph, GraphQL+- uses a widely-accepted [RDF N-Quad format](https://www.w3.org/TR/n-quads/).
Simply put, RDF N-Quad represents a fact. Let's understand this with a few examples.
```
<0x01> <name> "Alice" .
```

Here, `0x01` is the internal unique id assigned to an entity.
`name` is the predicate (relationship). This is the connector between the entity (person) and her name.
And finally, we have `"Alice"`, which is a string value representing her name.
RDF N-Quad ends with a `dot (.)`, to represent the end of a single fact.

### Language

RDF N-Quad allows specifying the language for string values, using `@lang`. Using that syntax, we can set `0x01`'s name in other languages.
```
<0x01> <name> "Алисия"@ru .
<0x01> <name> "Adélaïde"@fr .
```
To specify the language of the value to be returned from query `@lang1:lang2:lang3` notation is used. It is extension over RDF N-Quad syntax, and allows specifying multiple languages in order of preference. If value in given language is not found, next language from list is considered. If there are no values in any of specified languages, the value without specified language is returned. At last, if there is no value without language, value in ''some'' language is returned (this is implementation specific).

{{% notice "note" %}}Languages preference list cannot be used in functions.{{% /notice %}}

### Batch mutations

You can put multiple RDF lines into a single query to Dgraph. This is highly recommended.
Dgraph loader by default batches a 1000 RDF lines into one query, while running 100 such queries in parallel.

```
curl localhost:8080/query -XPOST -d $'
mutation {
  set {
    <0x01> <name> "Alice" .
    <0x01> <name> "Алисия"@ru .
    <0x01> <name> "Adélaïde"@fr .
  }
}' | python -m json.tool | less
```

### Assigning UID

Blank nodes (`_:<identifier>`) in mutations let you create a new entity in the database by assigning it a UID.
Dgraph can assign multiple UIDs in a single query while preserving relationships between these entities.
Let us say you want to create a database of students of a class using Dgraph, you could create new person entity using this method.
Consider the example below which creates a class, students and some relationships.

```
curl localhost:8080/query -XPOST -d $'
mutation {
 set {
    _:class <student> _:x .
    _:class <name> "awesome class" .
    _:x <name> "alice" .
    _:x <planet> "Mars" .
    _:x <friend> _:y .
    _:y <name> "bob" .
 }
}' | python -m json.tool | less
```

```
# This on execution returns
{
  "code": "Success",
  "message": "Done",
  "uids": {
    "class": "0x6bc818dc89e78754",
    "x": "0xc3bcc578868b719d",
    "y": "0xb294fb8464357b0a"
  }
}
# Three new entitities have been assigned UIDs.
```

The result of the mutation has a field called `uids` which contains the assigned UIDs.
The mutation above resulted in the formation of three new entities `class`, `x` and `y` which could be used by the user for later reference.
Now we can query as follows using the `_uid_` of `class`.

```
curl localhost:8080/query -XPOST -d $'{
 class(id:<assigned-uid>) {
  name
  student {
   name
   planet
   friend {
    name
   }
  }
 }
}' | python -m json.tool
```

```
# This query on execution results in
{
  "class": {
    "name@en": "awesome class",
    "student": {
      "friend": {
        "name@en": "bob"
      },
      "name@en": "alice",
      "planet": "Mars"
    }
  }
}
```

### External IDs (_xid_)

While not recommended, Dgraph supports directly using external ids in queries.
These could be useful if you are exporting existing data to Dgraph.
You could rewrite the above example as follows:
```
<alice> <name> "Alice" .
```

This would use a deterministic fingerprint of the XID `alice` and map that to a UID.

{{% notice "warning" %}}Dgraph does not store a UID to XID mapping. So, given a UID, it won't locate the corresponding XID automatically. Also, XID collisions are possible and are NOT checked for. If two XIDs fingerprint map to the same UID, their data would get mixed up.{{% /notice %}}

You can add and query the above data like so.
```
curl localhost:8080/query -XPOST -d $'
mutation {
  set {
    <alice> <name> "Alice" .
    <lewis-carrol> <died> "1998" .
  }
}

query {
  me(id:alice) {
    name
  }
}' | python -m json.tool | less
```

### Delete

`delete` keyword lets you delete a specified `S P O` triple through a mutation. For example, if you want to delete the record `<lewis-carrol> <died> "1998" .`, you would do the following.

```
curl localhost:8080/query -XPOST -d $'
mutation {
  delete {
     <lewis-carrol> <died> "1998" .
  }
}' | python -m json.tool | less
```

Or you can delete it using the `_uid_` for `lewis-carrol` like

```
curl localhost:8080/query -XPOST -d $'
mutation {
  delete {
     <0xf11168064b01135b> <died> "1998" .
  }
}' | python -m json.tool | less
```

## Queries
{{% notice "note" %}}Most of the examples here are based on the 21million.rdf.gz file [located here](https://github.com/dgraph-io/benchmarks/blob/master/data/21million.rdf.gz). The geo-location queries are based on sf.tourism.gz file [located here](https://github.com/dgraph-io/benchmarks/blob/master/data/sf.tourism.gz).{{% /notice %}}

To try out these queries, you can download these files, run Dgraph and load the data like so.
```
dgraph
```

```
# Adding a schema
curl localhost:8080/query -XPOST -d $'
mutation {
  schema {
    director.film: uid @reverse .
    genre: uid @reverse .
    initial_release_date: date @index .
    rating: uid @reverse .
    country: uid @reverse .
    loc: geo @index .
    name: string @index .
  }
}' | python -m json.tool | less
```

```
# Loading up the data from within the directory that contains your data files.
dgraphloader -r 21million.rdf.gz,sf.tourism.gz
```
We can also load the schema with dgraphloader

```
dgraphloader -r 21million.rdf.gz,sf.tourism.gz -s golden.schema
```

Queries in GraphQL+- look very much like queries in GraphQL. You typically start with a node or a list of nodes, and expand edges from there.
Each `{}` block goes one layer deep.

```
curl localhost:8080/query -XPOST -d $'{
  me(id: m.06pj8) {
    name@en
    director.film  {
      genre {
        name@en
      }
      name@en
      initial_release_date
    }
  }
}' | python -m json.tool | less
```

```
{
  "me": [
  {
    "director.film": [
      {
        "genre": [
          {
            "name@en": "Costume Adventure"
          },
          {
            "name@en": "Adventure Film"
          },
          {
            "name@en": "Action/Adventure"
          },
          {
            "name@en": "Action Film"
          }
        ],
        "initial_release_date": "1984-05-22",
        "name@en": "Indiana Jones and the Temple of Doom"
      },
      ...
      ...
      ...
      {
        "genre": [
          {
            "name@en": "Drama"
          }
        ],
        "initial_release_date": "1985-12-15",
        "name@en": "The Color Purple"
      }
    ],
    "name@en": "Steven Spielberg"
  }
  ]
}
```

What happened above is that we start the query with an entity denoting Steven Spielberg (from Freebase data), then expand by two predicates `name@en`(name in English) which yields the value `Steven Spielberg`, and `director.film` which yields the entities of films directed by Steven Spielberg.

Then for each of these film entities, we expand by three predicates: `name@en` which yields the name of the film, `initial_release_date` which yields the film release date and `genre` which yields a list of genre entities. Each genre entity is then expanded by `name@en` to get the name of the genre.

If you want to use a list, then the query would look like:

{{< runnable >}}{
  me(id: [m.06pj8, m.0bxtg]) {
    name@en
    director.film  {
      genre {
        name@en
      }
      name@en
      initial_release_date
    }
  }
}
{{< /runnable >}}

The list can contain XIDs or UIDs or a combination of both.

To specify the language of the value to be returned from query `@lang1:lang2:lang3` notation is used. It is extension over RDF N-Quad syntax, and allows specifying multiple languages in order of preference. If value in given language is not found, next language from list is considered. If there are no values in any of specified languages, the value without specified language is returned. At last, if there is no value without language, value in ''some'' language is returned (this is implementation specific).

{{% notice "note" %}}Languages preference list cannot be used in functions.{{% /notice %}}

## Schema

{{% notice "note" %}}Schema file is not supported from v0.7.4 onwards. You can use the `/query` endpoint to [add/modify]({{< relref "#adding-or-modifying-schema" >}}) or retrieve the schema.{{% /notice %}}

Schema is used to specify the types of the predicates. This schema would be used for type checking and type coercion.

### Schema Types

The following types are supported by Dgraph.

| Dgraph Type | Go type |
| ------------|:--------|
|  `int`      | int64   |
|  `float`    | float   |
|  `string`   | string  |
|  `bool`     | bool    |
|  `id`       | string  |
|  `date`     | time.Time (RFC3339 format [Optional timezone] eg: 2006-01-02T15:04:05.999999999+10:00 or 2006-01-02T15:04:05.999999999)    |
|  `geo`      | [go-geom](https://github.com/twpayne/go-geom)    |
|  `uid`      | uint64  |

{{% notice "note" %}}uid type is used to denote objects though it internally uses uint64.{{% /notice %}}


```
# Sample schema
name: string .
age: int .
```

* Mutations only check the scalar types. For example, in the given schema, any mutation that sets age would be checked for being a valid integer, any mutation that sets name would be checked for being a valid string.
* The returned fields are of types specified in the schema (given they were specified, or else derived from first mutation).
* **If schema was not specified, the schema would be derived based on the first mutation for that predicate.**  The [rdf types]({{< relref "#rdf-types" >}}) present in the first mutation would be considered as the schema for the field. If no storage type is specified in rdf, then it would be treated as default type(Dgraph Type) and it is stored internally as string(Go Type).

### Indexing

`@index` keyword at the end of a scalar field declaration in the schema specifies that the predicate should be indexed. For example, if we want to index some fields, we should have a schema similar to the one below.
```
name: string @index .
age: int @index .
address: string @index .
dateofbirth: date @index .
health: float @index .
location: geo @index .
timeafterbirth:  datetime @index .
```

All the scalar types except uid type can be indexed in dgraph. In the above example, we use the default tokenizer for each data type. You can specify a different tokenizer by writing `@index(tokenizerName)`.

```
name: string @index(exact, term) .
age: int @index(int) .
address: string @index(term) .
dateofbirth: date @index(date) .
health: float @index(float) .
location: geo @index(geo) .
timeafterbirth:  datetime @index(datetime) .
```

The available tokenizers are currently `term, fulltext, exact, int, float, geo, date, datetime`. All of them except `exact` and `fulltext` are the default tokenizers for their respective data types. You can specify multiple indexes per predicate as shown for `name` in the above example.

{{% notice "note" %}}To be able to do filtering on a predicate, you must index it.{{% /notice %}}

#### String Indices
There are three types of string indices: `exact`, `term` and `fulltext`. Following table summarize usage of each index type.

| Index type |                          Usage                         |
|:----------:|:------------------------------------------------------:|
|   `exact`  | matching of entire value, regular expressions          |
|   `term`   | matching of terms/words                                |
| `fulltext` | matching with language specific stemming and stopwords |

#### Sortable Indices

Not all the indices establish a total order among the values that they index. So, in order to order based on the values or do inequality operations, the corresponding predicates must have a sortable index. The non-sortable indices that are curently present are `term`. All other indices are sortable. For example to sort by names or do any inequality operations on it, this line **must** be specified in schema.
```
name: string @index(exact) .
```

### Reverse Edges
Each graph edge is unidirectional. It points from one node to another. A lot of times,  you wish to access data in both directions, forward and backward. Instead of having to send edges in both directions, you can use the `@reverse` keyword at the end of a uid (entity) field declaration in the schema. This specifies that the reverse edge should be automatically generated. For example, if we want to add a reverse edge for `directed_by` predicate, we should have a schema as follows.

```
directed_by: uid @reverse .
```

This would add a reverse edge for each `directed_by` edge and that edge can be accessed by prefixing `~` with the original predicate, i.e. `~directed_by`.

In the following example, we find films that are directed by Steven Spielberg, by using the reverse edges of `directed_by`. Here is the sample query:
```
curl localhost:8080/query -XPOST -d $'{
  me(id: m.06pj8) {
    name@en
    ~directed_by(first: 5) {
      name@en
    }
  }
}' | python -m json.tool | less
```

The results are:
```
{
  "me":[
    {
      "name@en":"Steven Spielberg",
      "~directed_by":[
        {
          "name@en":"Indiana Jones and the Temple of Doom"
        },
        {
          "name@en":"Jaws"
        },
        {
          "name@en":"Saving Private Ryan"
        },
        {
          "name@en":"Close Encounters of the Third Kind"
        },
        {
          "name@en":"Catch Me If You Can"
        }
      ]
    }
  ]
}
```
### Adding or Modifying Schema

We can add or modify the schema by specfiying the schema inside mutation block. We don't verify whether the already stored values are compatible with schema or not. We will try to convert the already stored values to schema type and return on query, or else we would ignore them on conversion failure.

```
curl localhost:8080/query -XPOST -d $'
mutation {
  schema {
    genre: uid @reverse .
  }
}' | python -m json.tool | less

```

Based on the given schema mutation, the query blocks until the index/reverse edges are rebuilt.

### Fetching Schema

Schema can be fetched using schema block inside query. Required fields can be specified inside schema block (type, index, reverse or tokenizer).

{{< runnable >}}schema {
  type
  index
  reverse
  tokenizer
}
{{< /runnable >}}

We can also specify the list of predicates for which we need the schema.

{{< runnable >}}schema(pred: [name, friend]) {
  type
  index
  reverse
  tokenizer
}
{{< /runnable >}}

## RDF Types
RDF types can also be used to specify the type of values. They can be attached to the values using the `^^` separator.

```
curl localhost:8080/query -XPOST -d $'
mutation {
 set {
  _:a <name> "Alice" .
  _:a <age> "15"^^<xs:int> .
  _:a <health> "99.99"^^<xs:float> .
 }
}' | python -m json.tool | less
```
This implies that name be stored as string(default), age as int and health as float.

{{% notice "note" %}}RDF type overwrites [schema type]({{< relref "#scalar-types" >}}) in case both are present. If both the RDF type and the schema type is missing, value is assumed to be of default type which is stored as string internally.{{% /notice %}}

### Supported

The following table lists all the supported [RDF datatypes](https://www.w3.org/TR/rdf11-concepts/#section-Datatypes) and the corresponding internal type format in which the data is stored.

| Storage Type | Dgraph type |
| -------------|:------------:|
|  <xs:string> | String |
|  <xs:dateTime> |                               DateTime |
|  <xs:date> |                                   Date |
|  <xs:int> |                                    Int |
|  <xs:boolean> |                                Bool |
|  <xs:double> |                                 Float |
|  <xs:float> |                                  Float |
|  <geo:geojson> |                               Geo |
|  <http://www.w3.org/2001/XMLSchema#string> |   String |
|  <http://www.w3.org/2001/XMLSchema#dateTime> | DateTime |
|  <http://www.w3.org/2001/XMLSchema#date> |     Date |
|  <http://www.w3.org/2001/XMLSchema#int> |      Int |
|  <http://www.w3.org/2001/XMLSchema#boolean> |  Bool |
|  <http://www.w3.org/2001/XMLSchema#double> |   Float |
|  <http://www.w3.org/2001/XMLSchema#float> |    Float |


In case a predicate has different schema type and storage type, the convertibility between the two is ensured during mutation and an error is thrown if they are incompatible.  The values are always stored as storage type if specified, or else they are converted to schema type and stored. Storage type is property of the value we are storing and schema type is property of the edge.

Example: If schema type is int and we do the following mutations.

```
<a> age "13" .
<b> age "13"^^<xs:string> .
<c> age "14.5"^^<xs:string> .
<d> age "14.5"
```

For each nquad we will try to convert the value to integer. Since no storage type is specified for a, it would be converted to int and stored. For b we will store the age as string after checking that it can be parsed to integer. Age for c and d won't be stored because the values are not compatible with type schema type(integer).

### Extended

We also support extended types besides RDF types.

#### Password type

Password for an entity can be set using the password type, and those attributes can not be queried directly but only be checked for a match using a function which is used to do the password verification.

To set password for a given node you need to add `^^<pwd:password>`, as below
```
curl localhost:8080/query -XPOST -d $'
mutation {
  set {
    <m.06pj8> <password> "JailBreakers"^^<pwd:password>     .
  }
}' | python -m json.tool | less
```

And later to check if some value is a correct password, you'd use a `checkpwd` function which would return a boolean value, true meaning the value was right and flase otherwise.
```
curl localhost:8080/query -XPOST -d $'{
  director(id:m.06pj8) {
    name@en
    checkpwd(password, "JailBreakers")
  }
}' | python -m json.tool | less
```

Output:
```
{
  "director": [
    {
      "name@en": "Steven Spielberg",
      "password": [
        {
          "checkpwd": true
        }
      ]
    }
  ]
}
```

## Pagination
Often there is too much data and you only want a slice of the data.

### First

If you want just the first few results as you expand the predicate for an entity, you can use the `first` argument. The value of `first` can be positive for the first N results, and ''negative for the last N.''

{{% notice "note" %}}Without a sort order specified, the results are sorted by `_uid_`, which is assigned randomly. So the ordering while deterministic, might not be what you expected.{{% /notice  %}}

This query retrieves the first two films directed by Steven Spielberg and their last 3 genres.

```
curl localhost:8080/query -XPOST -d $'{
  me(id: m.06pj8) {
    director.film (first: 2) {
      name@en
      initial_release_date
      genre (first: -3) {
          name@en
      }
    }
  }
}' | python -m json.tool | less
```

```
{
    "me": [
        {
            "director.film": [
                {
                    "genre": [
                        {
                            "name@en": "Adventure Film"
                        },
                        {
                            "name@en": "Action/Adventure"
                        },
                        {
                            "name@en": "Action Film"
                        }
                    ],
                    "initial_release_date": "1984-05-23",
                    "name@en": "Indiana Jones and the Temple of Doom"
                },
                {
                    "genre": [
                        {
                            "name@en": "Mystery film"
                        },
                        {
                            "name@en": "Horror"
                        },
                        {
                            "name@en": "Thriller"
                        }
                    ],
                    "initial_release_date": "1975-06-20",
                    "name@en": "Jaws"
                }
            ]
        }
    ]
}
```

`first` can also be specified at root.

### Offset

If you want the '''next''' one result, you want to skip the first two results with `offset:2` and keep only one result  with `first:1`.

```
curl localhost:8080/query -XPOST -d $'{
  me(id: m.06pj8) {
    name@en
    director.film(first:1, offset:2)  {
      genre {
        name@en
      }
      name@en
      initial_release_date
    }
  }
}' | python -m json.tool | less
```


Notice the `first` and `offset` arguments. Here is the output which contains only one result.

```
{
  "me": [
    {
      "director.film": [
        {
          "genre": [
            {
              "name@en": "War film"
            },
            {
              "name@en": "Drama"
            },
            {
              "name@en": "Action Film"
            }
          ],
          "initial_release_date": "1998-07-23",
          "name@en": "Saving Private Ryan"
        }
      ],
      "name@en": "Steven Spielberg"
    }
  ]
}
```

`offset` can also be specified at root to skip over some results.

### After

Dgraph assigns `uint64`'s to all entities which are called UIDs (unique internal IDs). All results are sorted by UIDs by default. Therefore, another way to get the next one result after the first two results is to specify that the UIDs of all results are larger than the UID of the second result.

This helps in pagination where the first query would be of the form `<attribute>(after: 0x0, first: N)` and the subsequent ones will be of the form `<attribute>(after: <uid of last entity in last result>, first: N)`

In the above example, the first two results are the film entities of "Indiana Jones and the Temple of Doom" and "Jaws". You can obtain their UIDs by adding the predicate `_uid_` in the query.

```
curl localhost:8080/query -XPOST -d $'{
  me(id: m.06pj8) {
    name@en
    director.film(first:2) {
      _uid_
      name@en
    }
  }
}' | python -m json.tool | less
```


The response looks like:
```
{
  "me": [
    {
      "director.film": [
        {
          "_uid_": "0xc17b416e58b32bb",
          "name@en": "Indiana Jones and the Temple of Doom"
        },
        {
          "_uid_": "0xc6f4b3d7f8cbbad",
          "name@en": "Jaws"
        }
      ],
      "name@en": "Steven Spielberg"
    }
  ]
}
```

Now we know the UID of the second result is `0xc6f4b3d7f8cbbad`. We can get the next one result by specifying the `after` argument.

```
curl localhost:8080/query -XPOST -d $'{
  me(id: m.06pj8) {
    name@en
    director.film(first:1, after:0xc6f4b3d7f8cbbad)  {
      genre {
        name@en
      }
      name@en
      initial_release_date
    }
  }
}' | python -m json.tool | less
```

The response is the same as before when we use `offset:2` and `first:1`.

```
{
    "me": [
        {
            "director.film": [
                {
                    "genre": [
                        {
                            "name@en": "War film"
                        },
                        {
                            "name@en": "Drama"
                        },
                        {
                            "name@en": "Action Film"
                        }
                    ],
                    "initial_release_date": "1998-07-24",
                    "name@en": "Saving Private Ryan"
                }
            ],
            "name@en": "Steven Spielberg"
        }
    ]
}
```

## Alias

Alias lets us provide alternate names to predicates in results for convenience.

For example, the following query replaces the predicate `name` with `full_name` in the JSON result.
```
curl localhost:8080/query -XPOST -d $'{
  me(id: m.0bxtg) {
    full_name:name@en
  }
}' | python -m json.tool | less
```

```
{
  "me":[
    {
      "full_name":"Tom Hanks"
    }
  ]
}
```

## Count

`count` function lets us obtain the number of entities instead of retrieving the entire list. For example, the following query
retrieves the name and the number of films acted by an actor with `_xid_` m.0bxtg.

```
curl localhost:8080/query -XPOST -d $'{
  me(id: m.0bxtg) {
    name@en
    count(actor.film)
  }
}' | python -m json.tool | less
```

```
{
 "me": [
  {
   "actor.film": [
     {
       "count": 75
      }
    ],
    "name@en": "Tom Hanks"
  }
 ]
}
```

## Functions

{{% notice "note" %}}Functions can only be applied to [indexed attributes]({{< relref "#indexing">}}).{{% /notice %}}

### Term matching

#### AllOfTerms

`allofterms` function will search for entities which have all of one or more terms specified. In essence, this is an intersection of entities containing the specified terms; the ordering does not matter. It follows this syntax: `allofterms(predicate, "space-separated terms")`

##### Usage as Filter

Suppose we want the films of Steven Spielberg that contain the word `indiana` and `jones`.
```
curl localhost:8080/query -XPOST -d $'{
  me(id: m.06pj8) {
    name@en
    director.film @filter(allofterms(name, "jones indiana"))  {
      name@en
    }
  }
}' | python -m json.tool | less
```

`allofterms` tells Dgraph that the matching films' `name` have to contain both the words "indiana" and "jones". Here is the response.

```
{
  "me": [
    {
      "director.film": [
        {
          "_uid_": "0xc17b416e58b32bb",
          "name@en": "Indiana Jones and the Temple of Doom"
        },
        {
          "_uid_": "0x7d0807a6740c25dc",
          "name@en": "Indiana Jones and the Kingdom of the Crystal Skull"
        },
        {
          "_uid_": "0xa4c4cc65751e98e7",
          "name@en": "Indiana Jones and the Last Crusade"
        },
        {
          "_uid_": "0xd1c161bed9769cbc",
          "name@en": "Indiana Jones and the Raiders of the Lost Ark"
        }
      ],
      "name@en": "Steven Spielberg"
    }
  ]
}
```

##### Usage at root

In the following example, we list all the entities (in this case all films) that have both terms "jones" and "indiana". Moreover, for each entity, we query their film genre and names.

```
curl localhost:8080/query -XPOST -d $'{
  me(func:allofterms(name, "jones indiana")) {
    name@en
    genre {
      name@en
    }
  }
}' | python -m json.tool | less
```

Here is a part of the response.

```
{
  "me": [
    {
      "genre": [
        {
          "name@en": "Adventure Film"
        },
        {
          "name@en": "Horror"
        }
      ],
      "name@en": "The Adventures of Young Indiana Jones: Masks of Evil"
    },
    {
      "genre": [
        {
          "name@en": "War film"
        },
        {
          "name@en": "Adventure Film"
        }
      ],
      "name@en": "The Adventures of Young Indiana Jones: Adventures in the Secret Service"
    },
    ...
    {
      "genre": [
        {
          "name@en": "Comedy"
        }
      ],
      "name@en": "The Adventures of Young Indiana Jones: Espionage Escapades"
    }
  ]
}
```

#### AnyOfTerms

`anyofterms` function will search for entities which have any of two or more terms specified. In essence, this is a union of entities containing the specified terms. Again, the ordering does not matter. It follows this syntax: `anyofterms(predicate, "space-separated terms")`

##### Usage as filter

```
curl localhost:8080/query -XPOST -d $'{
  me(id: m.06pj8) {
    name@en
    director.film @filter(anyofterms(name, "war spies"))  {
      _uid_
      name@en
    }
  }
}' | python -m json.tool | less
```


```
{
  "me": [
    {
      "director.film": [
        {
          "_uid_": "0x38160fa42cf3f4c9",
          "name@en": "War Horse"
        },
        {
          "_uid_": "0x39d8574f26521fcc",
          "name@en": "War of the Worlds"
        },
        {
          "_uid_": "0xd915cb0eb9ad47c0",
          "name@en": "Bridge of Spies"
        }
      ],
      "name@en": "Steven Spielberg"
    }
  ]
}
```

##### Usage at root

We can look up films that contain either the word "passion" or "peacock". Surprisingly many films satisfy this criteria. We will query their name and their genres.
```
curl localhost:8080/query -XPOST -d $'{
  me(func:anyofterms(name, "passion peacock")) {
    name@en
    genre {
      name@en
    }
  }
}' | python -m json.tool | less
```


```
{
  "me": [
    {
      "name@en": "Unexpected Passion"
    },
    {
      "genre": [
        {
          "name@en": "Drama"
        },
        {
          "name@en": "Silent film"
        }
      ],
      "name@en": "The New York Peacock"
    },
    {
      "genre": [
        {
          "name@en": "Drama"
        },
        {
          "name@en": "Romance Film"
        }
      ],
      "name@en": "Passion of Love"
    },
    ...
    {
      "genre": [
        {
          "name@en": "Crime Fiction"
        },
        {
          "name@en": "Comedy"
        }
      ],
      "name@en": "The Passion of the Reefer"
    }
  ]
}
```
Note that the first result with the name "Unexpected Passion" is either not a film entity, or it is a film entity with no genre.

### Regex search
`regexp` function allows a regular expression match on the values. It requires exact index to be present for working.

```
curl localhost:8080/query -XPOST -d $'{
  directors(func: regexp(name, "^Steven Sp.*$")) {
    name@en
    director.film @filter(regexp(name, "Ryan")) {
      name@en
    }
  }
}
'
```
Output:
```
{
  "directors": [
    {
      "director.film": [
        {
          "name@en": "Saving Private Ryan"
        }
      ],
      "name@en": "Steven Spielberg"
    },
    {
      "name@en": "Steven Spurrier"
    },
    {
      "name@en": "Steven Spurrier"
    },
    {
      "name@en": "Steven Spencer"
    },
    {
      "name@en": "Steve Spurrier"
    },
    {
      "name@en": "Steven Spieldal"
    },
    {
      "name@en": "Steven Spielberg And The Return To Film School"
    },
    {
      "name@en": "Steven Sperling"
    },
    {
      "name@en": "Steven Spohn"
    },
    {
      "name@en": "Steven Spielberg"
    },
    {
      "name@en": "Steven Spielberg"
    },
    {
      "name@en": "Steven Spencer"
    },
    {
      "name@en": "Steven Spielberg"
    }
  ]
}
```

{{% notice "note" %}}Regex function requires full index scan as of v0.7.4 which is slow. So it's recommended to use them only if absolutely necessary {{% /notice %}}

### Full Text Search
There are two functions for full text search - `alloftext` and `anyoftext`.
Both can be used similar to their pattern matching counterparts (`allofterms`, `anyofterms`).
Following steps are executed to process full text search function arguments:

1. Tokenization (according to Unicode word boundaries).
1. Conversion to lowercase.
1. Unicode-normalization (to [Normalization Form KC](http://unicode.org/reports/tr15/#Norm_Forms)).
1. Stemming using language-specific stemmer.
1. Stop words removal (language-specific).

The same steps are invoked during `fulltext` indexing of values.

Following table contains all supported languages and corresponding country-codes:

| Language    | Country Code |
|:-----------:|:------------:|
| Danish      | da           |
| Dutch       | nl           |
| English     | en           |
| Finnish     | fi           |
| French      | fr           |
| German      | de           |
| Hungarian   | hu           |
| Italian     | it           |
| Norwegian   | no           |
| Portuguese  | pt           |
| Romanian    | ro           |
| Russian     | ru           |
| Spanish     | es           |
| Swedish     | sv           |
| Turkish     | tr           |


### Inequality
#### Type Values
The following [Scalar_Types]({{<relref "#scalar-types">}}) can be used in inequality functions.

* int
* float
* string
* date
* datetime

#### Less than or equal to
`leq` is used to filter or obtain UIDs whose value for a predicate is less than or equal to a given value.

```
curl localhost:8080/query -XPOST -d $'{
  me(id: m.06pj8) {
    name@en
    director.film @filter(leq(initial_release_date, "1970-01-01"))  {
      initial_release_date
      name@en
    }
  }
}' | python -m json.tool | less
```

This query would return the name and release date of all the movies directed by on or Steven Spielberg before 1970-01-01.
```
{
    "me": [
        {
            "director.film": [
                {
                    "initial_release_date": "1964-03-24",
                    "name@en": "Firelight"
                },
                {
                    "initial_release_date": "1968-12-18",
                    "name@en": "Amblin"
                },
                {
                    "initial_release_date": "1967-01-01",
                    "name@en": "Slipstream"
                }
            ],
            "name@en": "Steven Spielberg"
        }
    ]
}
```

#### Greater than or equal to
```
curl localhost:8080/query -XPOST -d $'{
  me(id: m.06pj8) {
    name@en
    director.film @filter(geq(initial_release_date, "2008"))  {
      Release_date: initial_release_date
      Name: name@en
    }
  }
}' | python -m json.tool | less
```

This query would return Name and Release date of movies directed by Steven Spielberg after 2010.
```
{
    "me": [
        {
            "director.film": [
                {
                    "Name": "War Horse",
                    "Release_date": "2011-12-04"
                },
                {
                    "Name": "Indiana Jones and the Kingdom of the Crystal Skull",
                    "Release_date": "2008-05-18"
                },
                {
                    "Name": "Lincoln",
                    "Release_date": "2012-10-08"
                },
                {
                    "Name": "Bridge of Spies",
                    "Release_date": "2015-10-16"
                },
                {
                    "Name": "The Adventures of Tintin: The Secret of the Unicorn",
                    "Release_date": "2011-10-23"
                }
            ],
            "name@en": "Steven Spielberg"
        }
    ]
}
```

#### Less than, greater than, equal to

Above, we have seen the usage of `geq` and `leq`. You can also use `gt` for "strictly greater than" and `lt` for "strictly less than" and `eq` for "equal to".

### Geolocation
{{% notice "note" %}}Geolocation functions support only polygons and points as of now. Also, polygons with holes are replaced with the outer loop ignoring any holes.  {{% /notice %}}

The data used for testing the geo functions can be found in [benchmarks repository](https://github.com/dgraph-io/benchmarks/blob/master/data/sf.tourism.gz). You will need to [Index]({{< relref "#indexing" >}}) `loc` predicate with type `geo` before loading the data for these queries to work.
#### Near

`Near` returns all entities which lie within a specified distance from a given point. It takes in three arguments namely
the predicate (on which the index is based), geo-location point and a distance (in metres).

```
curl localhost:8080/query -XPOST -d $'{
  tourist(func: near(loc, [-122.469829, 37.771935], 1000) ) {
    name
  }
}' | python -m json.tool | less
```

This query returns all the entities located within 1000 metres from the [specified point](http://bl.ocks.org/d/2ba9f626cb7be1bcc012be1dc7db40ff) in geojson format.
```
{
    "tourist": [
        {
            "name": "National AIDS Memorial Grove"
        },
        {
            "name": "Japanese Tea Garden"
        },
        {
            "name": "Peace Lantern"
        },
        {
            "name": "Steinhart Aquarium"
        },
        {
            "name": "De Young Museum"
        },
        {
            "name": "Morrison Planetarium"
        },
         .
         .
        {
            "name": "San Francisco Botanical Garden"
        },
        {
            "name": "Buddha"
        }
    ]
}
```

#### Within

`Within` returns all entities which completely lie within the specified region. It takes in two arguments namely the predicate (on which the index is based) and geo-location region.

```
curl localhost:8080/query -XPOST -d $'{
  tourist(func: within(loc, [[-122.47266769409178, 37.769018558337926 ], [ -122.47266769409178, 37.773699921075135 ], [ -122.4651575088501, 37.773699921075135 ], [ -122.4651575088501, 37.769018558337926 ], [ -122.47266769409178, 37.769018558337926]] )) {
    name
  }
}' | python -m json.tool | less
```
This query returns all the entities (points/polygons) located completely within the [specified polygon](http://bl.ocks.org/d/b81a6589fa9639c9424faad778004dae) in geojson format.
```
{
    "tourist": [
        {
            "name": "Japanese Tea Garden"
        },
        {
            "name": "Peace Lantern"
        },
        {
            "name": "Rose Garden"
        },
        {
            "name": "Steinhart Aquarium"
        },
        {
            "name": "De Young Museum"
        },
        {
            "name": "Morrison Planetarium"
        },
        {
            "name": "Spreckels Temple of Music"
        },
        {
            "name": "Hamon Tower"
        },
        {
            "name": "Buddha"
        }
    ]
}
```
{{% notice "note" %}}The containment check for polygons are approximate as of v0.7.1.{{% /notice %}}

#### Contains

`Contains` returns all entities which completely enclose the specified point or region. It takes in two arguments namely the predicate (on which the index is based) and geo-location region.

```
curl localhost:8080/query -XPOST -d $'{
  tourist(func: contains(loc, [ -122.50326097011566, 37.73353615592843 ] )) {
    name
  }
}
```
This query returns all the entities that completely enclose the [http://bl.ocks.org/d/7218dd34391fac518e3516ea6fc1b6b1 specified point] (or polygon) in geojson format.
```
{
    "tourist": [
        {
            "name": "San Francisco Zoo"
        },
        {
            "name": "Flamingo"
        }
    ]
}
```

#### Intersects

`Intersects` returns all entities which intersect with the given polygon. It takes in two arguments namely the predicate (on which the index is based) and geo-location region.

```
curl localhost:8080/query -XPOST -d $'{
  tourist(func: intersects(loc, [[-122.503325343132, 37.73345766902749 ], [ -122.503325343132, 37.733903134117966 ], [ -122.50271648168564, 37.733903134117966 ], [ -122.50271648168564, 37.73345766902749 ], [ -122.503325343132, 37.73345766902749]] )) {
    name
  }
}
```
This query returns all the entities that intersect with the [http://bl.ocks.org/d/2ed3361a25442414e15d7eab88574b67 specified polygon/point] in geojson format.
```
{
    "tourist": [
        {
            "name": "San Francisco Zoo"
        },
        {
            "name": "Flamingo"
        }
    ]
}
```

## Filters

[Functions]({{< relref "#functions" >}}) can be applied to results as Filters.

#### AND, OR and NOT

Dgraph supports AND, OR and NOT filters. The syntax is of form: `A OR B`, `A AND B`, or `NOT A`. You can add round brackets to make these filters more complex. `(NOT A OR B) AND (C AND NOT (D OR E))`

In this query, we are getting film names which contain either both "indiana" and "jones" OR both "jurassic" AND "park".
```
curl localhost:8080/query -XPOST -d $'{
  me(id: m.06pj8) {
    name@en
    director.film @filter(allofterms(name, "jones indiana") OR allofterms(name, "jurassic park"))  {
      _uid_
      name@en
    }
  }
}' | python -m json.tool | less
```


```
{
  "me": [
    {
      "director.film": [
        {
          "_uid_": "0xc17b416e58b32bb",
          "name@en": "Indiana Jones and the Temple of Doom"
        },
        {
          "_uid_": "0x22e65757df0c94d2",
          "name@en": "Jurassic Park"
        },
        {
          "_uid_": "0x7d0807a6740c25dc",
          "name@en": "Indiana Jones and the Kingdom of the Crystal Skull"
        },
        {
          "_uid_": "0x8f2485e4242cbe6e",
          "name@en": "The Lost World: Jurassic Park"
        },
        {
          "_uid_": "0xa4c4cc65751e98e7",
          "name@en": "Indiana Jones and the Last Crusade"
        },
        {
          "_uid_": "0xd1c161bed9769cbc",
          "name@en": "Indiana Jones and the Raiders of the Lost Ark"
        }
      ],
      "name@en": "Steven Spielberg"
    }
  ]
}
```

#### Filter on Compare

Dgraph also supports compare filter, which takes form as @filter(compare(count(attribute), N)), only entities fulfill such predication will be returned.

{{% notice "note" %}}"Compare" here includes "eq", "gt", "geq", "lt", "leq".  And "count" should be applied on non-scalar types.{{% /notice %}}
```
curl localhost:8080/query -XPOST -d $'{
  director(func:anyofterms(name, "Steven Spielberg")) @filter(gt(count(director.film), 36)) {
    name@en
    count(director.film)
  }
}' | python -m json.tool | less
```

Output:

```
{
  "director": [
    {
      "director.film": [
        {
          "count": 39
        }
      ],
      "name@en": "Steven Spielberg"
    },
    {
      "director.film": [
        {
          "count": 115
        }
      ],
      "name@en": "Steven Scarborough"
    }
  ]
}
```

These compare functions can also be used at root. To obtain one movie per genre which have atleast 30000 movies, we'd do as follows:

```
curl localhost:8080/query -XPOST -d $'{
	genre(func: gt(count(~genre), 30000)){
		name@en
		~genre (first:1) {
			name@en
		}
	}
}'
```
Output:
```
{
  "genre": [
    {
      "name@en": "Short Film",
      "~genre": [
        {
          "name@en": "Eine Rolle Duschen"
        }
      ]
    },
    {
      "name@en": "Drama",
      "~genre": [
        {
          "name@en": "Prisoners"
        }
      ]
    },
    {
      "name@en": "Comedy",
      "~genre": [
        {
          "name@en": "A tu per tu"
        }
      ]
    },
    {
      "name@en": "Documentary film",
      "~genre": [
        {
          "name@en": "Short Cut to Nirvana: Kumbh Mela"
        }
      ]
    }
  ]
}
```
These functions can help is starting from nodes which have some conditions based on count and might help in determining the type of a node if modelled accordingly. For example, to start with all the directors, we can do `gt(count(director.film), 0)` which means all the nodes that have alteast one outgoing `director.film` edge.

## Sorting

We can sort results by a predicate using the `orderasc` or `orderdesc` argument. The predicate has to be indexed and this has to be specified in the schema. As you may expect, `orderasc` sorts in ascending order while `orderdesc` sorts in descending order.

For example, we can sort the films of Steven Spielberg by their release date, in ascending order.
```
curl localhost:8080/query -XPOST -d $'{
  me(id: m.06pj8) {
    name@en
    director.film(orderasc: initial_release_date) {
      name@en
      initial_release_date
    }
  }
}' | python -m json.tool | less
```

```
{
  "me": [
    {
      "director.film": [
        {
          "initial_release_date": "1964-03-23",
          "name@en": "Firelight"
        },
        {
          "initial_release_date": "1966-12-31",
          "name@en": "Slipstream"
        },
        {
          "initial_release_date": "1968-12-17",
          "name@en": "Amblin"
        },
        ...
        ...
        ...
        {
          "initial_release_date": "2012-10-07",
          "name@en": "Lincoln"
        },
        {
          "initial_release_date": "2015-10-15",
          "name@en": "Bridge of Spies"
        }
      ],
      "name@en": "Steven Spielberg"
    }
  ]
}
```

If you use `orderdesc` instead, the films will be listed in descending order.
```
curl localhost:8080/query -XPOST -d $'{
  me(id: m.06pj8) {
    name@en
    director.film(orderdesc: initial_release_date, first: 2) {
      name@en
      initial_release_date
    }
  }
}' | python -m json.tool | less
```


Here is the output.

```
{
  "me": [
    {
      "director.film": [
        {
          "initial_release_date": "2015-10-15",
          "name@en": "Bridge of Spies"
        },
        {
          "initial_release_date": "2012-10-07",
          "name@en": "Lincoln"
        },
      ],
      "name@en": "Steven Spielberg"
    }
  ]
}
```

To sort at root level, we can do as follows:
```
curl localhost:8080/query -XPOST -d $'{
  me(func: allofterms(name, "ste"), orderasc: name) {
    name@en
  }
}' | python -m json.tool | less
```

```
{
    "me": [
        {
            "name@en": "Steven's Friend #1"
        },
        {
            "name@en": "Steven's Friend #2"
        },
        {
            "name@en": "Steven's Orderly #2"
        },
        {
            "name@en": "United Standards 2 (as Steven Carell)"
        },
        {
            "name@en": "American Boy: A Profile of Steven Prince"
        },
        {
            "name@en": "Steven A Burd"
....
 {
            "name@en": "Steven Kirshoff"
        },
        {
            "name@en": "Steven Kirtley"
        },
        {
            "name@en": "Steven Kissel"
        }
    ]
}
```


## Facets : Edge attributes

Dgraph support facets which are key value pairs on edges.
In a graph database, you can use facets for adding properties to edges, rather than to nodes.
For e.g. `friend` edge between two nodes may have property of `close` friendship.
Facets can also be used as `weights` for edges.

Though you may find yourself leaning towards facets many times, they should not be misused while modelling data.
Like `friend` edge should not have `date of birth`. That should be edge for object node of `friend`.
Another reason is that facets are not first class citizen in Dgraph like predicates (edges).

Keys are strings and Values can be string, bool, int, float and datetime.
For int and float, only decimal integer of upto 32 signed bits and 64 bit float values are accepted respectively.

Example : Adds `since` facet in `mobile` edge for `alice`:

```
curl localhost:8080/query -XPOST -d $'
mutation {
 set {
  <alice> <name> "alice" .
  <alice> <mobile> "040123456" (since=2006-01-02T15:04:05) .
  <alice> <car> "MA0123" (since=2006-02-02T13:01:09, first=true) .
 }
}' | python -m json.tool | less
```

Querying `name` and `mobile` of `alice` gives us :
```
curl localhost:8080/query -XPOST -d $'{
  data(id:alice) {
     name
     mobile
     car
  }
}' | python -m json.tool | less
```

Output:
```
{
  "data": [
    {
      "car": "MA0123",
      "mobile": "040123456",
      "name": "alice"
    }
  ]
}
```

We can ask for `facets` for `mobile` and `car` of `alice` using `@facets(since)`:
```
curl localhost:8080/query -XPOST -d $'{
  data(id:alice) {
     name
     mobile @facets(since)
     car @facets(since)
  }
}' | python -m json.tool | less
```

and we get `@facets` key at same level as that of `mobile` and `car`.
`@facets` map will have keys of `mobile` and `car` with their respective facets.

Output:
```
{
  "data": [
    {
      "@facets": {
        "car": {
          "since": "2006-02-02T13:01:09Z"
        },
        "mobile": {
          "since": "2006-01-02T15:04:05Z"
        }
      },
      "car": "MA0123",
      "mobile": "040123456",
      "name": "alice"
    }
  ]
}
```

You can also fetch all facets on an edge by simply using `@facets`.

```
curl localhost:8080/query -XPOST -d $'{
  data(id:alice) {
     name
     mobile @facets
     car @facets
  }
}' | python -m json.tool | less

```

Ouput:

```
{
  "data": [
    {
      "@facets": {
        "car": {
          "first": true,
          "since": "2006-02-02T13:01:09Z"
        },
        "mobile": {
          "since": "2006-01-02T15:04:05Z"
        }
      },
      "car": "MA0123",
      "mobile": "040123456",
      "name": "alice"
    }
  ]
}
```

Notice that you also get `first` under `car` key of `@facets` in this case.

### Facets on Uid edges

`friend` is an edge with facet `close`.
It is set to true for friendship between alice and bob
and false for friendship between alice and charlie.

```
curl localhost:8080/query -XPOST -d $'
mutation {
 set {
  <alice> <name> "alice" .
  <bob> <name> "bob" .
  <bob> <car> "MA0134" (since=2006-02-02T13:01:09) .
  <charlie> <name> "charlie" .
  <alice> <friend> <bob> (close=true) .
  <alice> <friend> <charlie> (close=false) .
 }
}' | python -m json.tool | less
```

If we query friends of `alice` we get :
```
curl localhost:8080/query -XPOST -d $'
{
  data(id:alice) {
    name
    friend {
      name
    }
  }
}' | python -m json.tool | less
```

Output :
```
{
  "data": [
    {
      "friend": [
        {
          "name": "bob"
        },
        {
          "name": "charlie"
        }
      ],
      "name": "alice"
    }
  ]
}
```

You can query back the facets with `@facets(close)` :

```
curl localhost:8080/query -XPOST -d $'{
   data(id:alice) {
     name
     friend @facets(close) {
       name
     }
   }
}' | python -m json.tool | less
```

This puts a key `@facets` in each of the child of `friend` in output result of previous query.
This keeps the relationship between which facet of `close` belongs of which child.
Since these facets come from parent, Dgraph uses key `_` to distinguish them from other
`facets` at child level.

```
{
  "data": [
    {
      "friend": [
        {
          "@facets": {
            "_": {
              "close": true
            }
          },
          "name": "bob"
        },
        {
          "@facets": {
            "_": {
              "close": false
            }
          },
          "name": "charlie"
        }
      ],
      "name": "alice"
    }
  ]
}
```

So, for uid edges like `friend`, facets go to the corresponding child's `@facets` under key `_`.

To see output for both facets on uid-edges (like friend) and value-edges (like car) :

```
curl localhost:8080/query -XPOST -d $'{
  data(id:alice) {
    name
    friend @facets {
      name
      car @facets
    }
  }
}' | python -m json.tool | less
```

Output:

```
{
  "data": [
    {
      "friend": [
        {
          "@facets": {
            "_": {
              "close": true
            },
            "car": {
              "since": "2006-02-02T13:01:09Z"
            }
          },
          "car": "MA0134",
          "name": "bob"
        },
        {
          "@facets": {
            "_": {
              "close": false
            }
          },
          "name": "charlie"
        }
      ],
      "name": "alice"
    }
  ]
}
```

Since, `bob` has a `car` and it has a facet `since`, it is part of same object as that of `bob`.
Also, `close` relationship between `bob` and `alice` is part of `bob`'s output object.
For `charlie` who does not have `car` edge and no facets other than that of friend's `close`.

### Filtering on facets

For the next query examples of facets, we use following data set :
```
curl localhost:8080/query -XPOST -d $'
  mutation {
    set {
      <alice> <name> "alice" .
      <bob> <name> "bob" .
      <charlie> <name> "charlie" .
      <dave> <name> "dave" .
      <alice> <friend> <bob> (close=true, relative=false) .
      <alice> <friend> <charlie> (close=false, relative=true) .
      <alice> <friend> <dave> (close=true, relative=true) .
    }
  }
' | python -m json.tool | less
```
On previous dataset, this adds `dave` as friend of `alice` and `relative` facet on all friend edges.

We support filtering edges based on facets. You can use all kinds of filtering functions like
`allofterms, geq, eq etc.` with facets.

Have a look at below example:
```
curl localhost:8080/query -XPOST -d $'{
  data(id:<alice>) {
    friend @facets(eq(close, true)) {
      name
    }
  }
}' | python -m json.tool | less
```

You can guess that above query give name of all friends which have `close` facet set to true.

Output : We should not get `charlie` as he is not alice's close friend.
```
{
  "data": [
    {
      "friend": [
        {
          "name": "bob"
        },
        {
          "name": "dave"
        }
      ]
    }
  ]
}
```

You can ask for facets while filtering on them by adding another `@facets(<facetname>)` to query.

```
curl localhost:8080/query -XPOST -d $'{
  data(id:<alice>) {
    friend @facets(eq(close, true)) @facets(relative) { # filter close friends and give relative status
      name
    }
  }
}' | python -m json.tool | less
```
Output : We should get `relative` in our `@facets`.
```
{
  "data": [
    {
      "friend": [
        {
          "@facets": {
            "_": {
              "relative": false
            }
          },
          "name": "bob"
        },
        {
          "@facets": {
            "_": {
              "relative": true
            }
          },
          "name": "dave"
        }
      ]
    }
  ]
}
```

Of course, You can use composition of filtering functions together like:
```
curl localhost:8080/query -XPOST -d $'{
  data(id:<alice>) {
    friend @facets(eq(close, true) AND eq(relative, true)) @facets(relative) { # filter close friends in my relation
      name
    }
  }
}' | python -m json.tool | less
```
Output : `dave` is only close friend who is also my relative.
```
{
  "data": [
    {
      "friend": [
        {
          "@facets": {
            "_": {
              "relative": true
            }
          },
          "name": "dave"
        }
      ]
    }
  ]
}
```

## Aggregation
Aggregation functions that are supported are `min, max, sum, avg`. While min and max operate on all scalar-values, sum and avg can operate only on `int and float` types. Aggregation does not depend on the index. All the aggregation results are attached one level above in the result.

{{% notice "note" %}}We support aggregation on scalar type only.{{% /notice %}}

### Min
```
curl localhost:8080/query -XPOST -d $'{
  director(id: m.06pj8) {
    director.film {
    	min(initial_release_date)
    }
  }
}' | python -m json.tool | less
```
Output:

```
{
  "director": [
    {
      "director.film": [
        {
          "min(initial_release_date)": "1964-03-24"
        }
      ]
    }
  ]
}
```

### Max
```
curl localhost:8080/query -XPOST -d $'{
  director(id: m.06pj8) {
    director.film {
    	max(initial_release_date)
    }
  }
}' | python -m json.tool | less
```

Output:

```
{
  "director": [
    {
      "director.film": [
        {
          "max(initial_release_date)": "2015-10-16"
        }
      ]
    }
  ]
}
```

### Sum, Avg
```
curl localhost:8080/query -XPOST -d $'
mutation {
 set {
	<0x01> <name> "Alice"^^<xs:string> .
	<0x02> <name> "Tom"^^<xs:string> .
	<0x03> <name> "Jerry"^^<xs:string> .
	<0x04> <name> "Teddy"^^<xs:string> .
	# friend of 0x01 Alice
	<0x01> <friend> <0x02> .
	<0x01> <friend> <0x03> .
	<0x01> <friend> <0x04> .
	# set age
	<0x02> <age> "99"^^<xs:int> .
	<0x03> <age> "100"^^<xs:int>	.
	<0x04> <age> "101"^^<xs:int>	.
 }
}
query {
	me(id:0x01) {
		friend {
			name
			age
			sum(age)
			avg(age)			
		}
	}
}' | python -m json.tool | less
```

Output:

```
{
    "me": [
        {
            "friend": [
                {
                    "age": 99,
                    "name": "Tom"
                },
                {
                    "age": 100,
                    "name": "Jerry"
                },
                {
                    "age": 101,
                    "name": "Teddy"
                },
                {
                    "sum(age)": 300
                },
                {
                    "avg(age)": 100.0
                }
            ]
        }
    ]
}
```

## Multiple Query Blocks
Multiple blocks can be inside a single query and they would be returned in the result with the corresponding block names.

```
curl localhost:8080/query -XPOST -d $'{
 AngelinaInfo(func:allofterms(name, "angelina jolie")) {
  name@en
   actor.film {
    performance.film {
      genre {
        name@en
      }
    }
   }
  }

 DirectorInfo(id: m.06pj8) {
    name@en
    director.film @filter(geq(initial_release_date, "2008"))  {
        Release_date: initial_release_date
        Name: name@en
    }
  }
}' | python -m json.tool | less
```


```
{
  "AngelinaInfo": [
    {
      "name@en": "Angelina Jolie Look-a-Like"
    },
    {
      "name@en": "Angelina Jolie"
    },
    {
      "actor.film": [
        {
          "performance.film": [
            {
              "genre": [
                {
                  "name@en": "Animation"
                },
                {
                  "name@en": "Fantasy"
                },
                {
                  "name@en": "Computer Animation"
                },
                {
                  "name@en": "Adventure Film"
                },
                {
                  "name@en": "Action Film"
                }
              ]
            }
          ]
        },
        {
          "performance.film": [
            {
              "genre": [
                {
                  "name@en": "Comedy"
                }
              ]
            }
          ]
        },

.
.
.
  "DirectorInfo": [
    {
      "director.film": [
        {
          "Name": "War Horse",
          "Release_date": "2011-12-04"
        },
        {
          "Name": "Indiana Jones and the Kingdom of the Crystal Skull",
          "Release_date": "2008-05-18"
        },
        {
          "Name": "Lincoln",
          "Release_date": "2012-10-08"
        },
        {
          "Name": "Bridge of Spies",
          "Release_date": "2015-10-16"
        },
        {
          "Name": "The Adventures of Tintin: The Secret of the Unicorn",
          "Release_date": "2011-10-23"
        }
      ],
      "name@en": "Steven Spielberg"
    }
  ]
}
```

### Var Blocks
Var blocks are the blocks which start with the keyword `var` and these blocks are not returned in the query results.

```
curl localhost:8080/query -XPOST -d $'{
 var(func:allofterms(name, "angelina jolie")) {
  name@en
   actor.film {
    A AS performance.film {
     B AS genre
    }
   }
  }

 films(id: var(B)) {
   ~genre @filter(var(A)) {
     name@en
   }
 }
}' | python -m json.tool | less
```

This query returns.
```
{
  "films": [
    {
      "~genre": [
        {
          "name@en": "Hackers"
        }
      ]
    },
    {
      "~genre": [
        {
          "name@en": "Mr. & Mrs. Smith"
        }
      ]
    },
    {
      "~genre": [
        {
          "name@en": "Hackers"
        },
        {
          "name@en": "Foxfire"
        }
      ]
    },
.
.
.
      {
          "name@en": "Without Evidence"
        },
        {
          "name@en": "True Women"
        },
        {
          "name@en": "Salt"
        }
      ]
    },
    {
      "~genre": [
        {
          "name@en": "Confessions of an Action Star"
        }
      ]
    },
    {
      "~genre": [
        {
          "name@en": "Without Evidence"
        }
      ]
    }
  ]
}
```

## Query Variables

Variables can be defined at different levels of the query using the keyword `AS` and would contain the result list at that level as its value. These variables can be used in other query blocks or the same block (as long as it is used in the child nodes).

```
curl localhost:8080/query -XPOST -d $'{
 var(func:allofterms(name, "angelina jolie")) {
  name@en
   actor.film {
    A AS performance.film {  # All the films done by Angelina Jolie
     B As genre  # Genres of all the films done by Angelina Jolie
    }
   }
  }

 var(func:allofterms(name, "brad pitt")) {
  name@en
   actor.film {
    C AS performance.film {  # All the films done by Brad Pitt
     D as genre  # Genres of all the films done by Brad Pitt
    }
   }
  }

 films(id: var(D)) @filter(var(B)) {   # movies done by both Angelina and Brad
  name@en
   ~genre @filter(var(A) OR var(C)) {  # Genres of movies done by Angelina or Brad
     name@en
   }
 }
}' | python -m json.tool | less
```

```
{
  "films": [
    {
      "~genre": [
        {
          "name@en": "Hackers"
        },
        {
          "name@en": "No Way Out"
        }
      ]
    },
    {
      "~genre": [
        {
          "name@en": "Being John Malkovich"
        },
        {
          "name@en": "Burn After Reading"
        },
        {
          "name@en": "Inglourious Basterds"
        },
        {
          "name@en": "Mr. & Mrs. Smith"
        }
      ]
    },
    {
      "~genre": [
        {
          "name@en": "Less Than Zero"
        },
        {
          "name@en": "Hackers"
        },
        {
          "name@en": "Foxfire"
.
.
.
     "name@en": "Salt"
        }
      ]
    },
    {
      "~genre": [
        {
          "name@en": "Johnny Suede"
        },
        {
          "name@en": "Confessions of an Action Star"
        }
      ]
    },
    {
      "~genre": [
        {
          "name@en": "Without Evidence"
        },
        {
          "name@en": "Too Young to Die?"
        }
      ]
    }
  ]
}

```

## Value Variables

Value variables are those which store the scalar values (unlike the UID lists which we saw above). These are a map from the UID to the corresponding value. They can store scalar predicates, aggregate functions, can be used for sorting resutls and retrieving. For example:

```
curl localhost:8080/query -XPOST -d $'{
	var(func:allofterms(name, "angelina jolie")) {
 		name@en
		actor.film {
			performance.film {
				B AS genre {
					A as name@en
				}
			}
		}
	}

	genre(id: var(B), orderasc: var(A)) @filter(gt(count(~genre), 30000)){
		var(A)
		~genre {
			min(name)
			max(name)
			min(initial_release_date)
			max(initial_release_date)
		}
	}
}' | python -m json.tool | less
```
Output:
```
{
  "genre": [
    {
      "var[A]": "Comedy",
      "~genre": [
        {
          "min(name)": "#1 Cheerleader Camp"
        },
        {
          "max(name)": "후라이보이 박사소동"
        },
        {
          "min(initial_release_date)": "0214-02-28"
        },
        {
          "max(initial_release_date)": "2103-01-01"
        }
      ]
    },
    {
      "var[A]": "Drama",
      "~genre": [
        {
          "min(name)": "#Stuck"
        },
        {
          "max(name)": "李朝 春花圖"
        },
        {
          "min(initial_release_date)": "0214-02-28"
        },
        {
          "max(initial_release_date)": "2017-12-01"
        }
      ]
    },
    {
      "var[A]": "Short Film",
      "~genre": [
        {
          "min(name)": "#11, MareyMoiré"
        },
        {
          "max(name)": "휴가"
        },
        {
          "min(initial_release_date)": "0214-02-28"
        },
        {
          "max(initial_release_date)": "2103-05-22"
        }
      ]
    }
  ]
}
```
This query shows a mix of how things can be used.

### Aggregating value variables

Currently we allow adding value variabels and assigning it to a new variable. This can be very useful if you want a simple recommendation system based on a formula. For example:

```
curl localhost:8080/query -XPOST -d $'{
	var(func:allofterms(name, "steven spielberg")) {
		name@en
		films as director.film {
			p as count(starring)
			q as count(genre)
			r as count(country)
			score as sumvar(p, q, r)
		}
	}

	TopMovies(id: var(films), orderdesc: var(score), first: 5){
		name@en
		var(score)
	}
}' | python -m json.tool | less
```
Output:
```
{
  "TopMovies": [
    {
      "name@en": "Lincoln",
      "var[score]": 179
    },
    {
      "name@en": "Minority Report",
      "var[score]": 156
    },
    {
      "name@en": "Schindler's List",
      "var[score]": 145
    },
    {
      "name@en": "The Terminal",
      "var[score]": 118
    },
    {
      "name@en": "Saving Private Ryan",
      "var[score]": 99
    }
  ]
}
```
In the above query we retrieve the top movies (by sum of number of actors, genres, countries) of the entity named steven spielberg.


## Shortest Path Queries

Shortest path between a `src` node and `dst` node can be found using the keyword `shortest` for the query block name. It requires the source node id, destination node id and the predicates (atleast one) that have to be considered for traversing. This query block by itself will not return any results back but the path has to be stored in a variable and used in other query blocks as required.

{{% notice "note" %}}If no predicates are specified in the `shortest` block, no path can be fetched as no edge is traversed.{{% /notice %}}

For example:
```
# Insert this via mutation
curl localhost:8080/query -XPOST -d $'
mutation{
set {
 <a> <friend> <b> (weight=0.1) .
 <b> <friend> <c> (weight=0.2) .
 <c> <friend> <d> (weight=0.3) .
 <a> <friend> <d> (weight=1) .
 <a> <name> "alice" .
 <b> <name> "bob" .
 <c> <name> "Tom" .
 <d> <name> "Mallory" .
 }
}' | python -m json.tool | less
```

```
curl localhost:8080/query -XPOST -d $'{
 path as shortest(from:a, to:d) {
  friend
 }
 path(id: var(path)) {
   name
 }
}' | python -m json.tool | less
```
Would return the following results. (Note that each edges' weight is considered as 1)
 ```
{
    "_path_": [
        {
            "_uid_": "0xb3454265b6df75e3",
            "friend": [
                {
                    "_uid_": "0x3e0ae463957d9a21"
                }
            ]
        }
    ],
    "path": [
        {
            "name": "alice"
        },
        {
            "name": "Mallory"
        }
    ]
}
```
If we want to use edge weights, we'd use facets to specify them as follows.

{{% notice "note" %}}We can specify exactly one facet per predicate in the shortest query block.{{% /notice %}}
```
curl localhost:8080/query -XPOST -d $'{
 path as shortest(from:a, to:d) {
  friend @facets(weight)
 }

 path(id: var(path)) {
  name
 }
}' | python -m json.tool | less
```

```
{
  "_path_": [
    {
      "_uid_": "0xb3454265b6df75e3",
      "friend": [
        {
          "@facets": {
            "_": {
              "weight": 0.1
            }
          },
          "_uid_": "0xa3b260215ec8f116",
          "friend": [
            {
              "@facets": {
                "_": {
                  "weight": 0.2
                }
              },
              "_uid_": "0x9ea118a9e0cb7b28",
              "friend": [
                {
                  "@facets": {
                    "_": {
                      weight": 0.3
                    }
                  },
                  "_uid_": "0x3e0ae463957d9a21"
                }
              ]
            }
          ]
        }
      ]
    }
  ],
  "path": [
    {
      "name": "alice"
    },
    {
      "name": "bob"
    },
    {
      "name": "Tom"
    },
    {
      "name": "Mallory"
    }
  ]
}

```

Another query which shows how to retrieve paths with some constraints on the intermediate nodes.
```
curl localhost:8080/query -XPOST -d $'{
  path as shortest(from: a, to: d) {
    friend @filter(not anyofterms(name, "bob")) @facets(weight)
    relative @facets(liking)
  }

  relationship(id: var(path)) {
    name
  }
}' | python -m json.tool | less
```

This query would again retrieve the shortest path but using some different parameters for the edge weights which are specified using facets (weight and liking). Also, we'd not like to have any person whose name contains `alice` in the path which is specified by the filter.

## Recurse Query

`Recurse` queries let you traverse a set of predicates (with filter, facets, etc.) until we reach all leaf nodes or we reach the maximum depth which is specified by the `depth` parameter.

To get 10 movies from a genre that has more than 30000 films and then get two actors for those movies we'd do something as follows:
```
curl localhost:8080/query -XPOST -d $'{
	recurse(func: gt(count(~genre), 30000), first: 1){
		name@en
		~genre (first:10) @filter(gt(count(starring), 2))
		starring (first: 2)
		performance.actor
	}
}'
```
Output:
```
{
  "recurse": [
    {
      "name@en": "Short Film",
      "~genre": [
        {
          "name@en": "Life Begins for Andy Panda",
          "starring": [
            {
              "performance.actor": [
                {
                  "name@en": "Bernice Hansen"
                }
              ]
            },
            {
              "performance.actor": [
                {
                  "name@en": "Mel Blanc"
                }
              ]
            }
          ]
        },
        {
          "name@en": "Liviu's Dream",
          "starring": [
            {
              "performance.actor": [
                {
                  "name@en": "Catalina Harabagiu"
                }
              ]
            },
            {
              "performance.actor": [
                {
                  "name@en": "Adrian Vancică"
                }
              ]
            }
          ]
        },
        {
          "name@en": "Payload",
          "starring": [
            {
              "performance.actor": [
                {
                  "name@en": "Roshan Johal"
                }
              ]
            },
            {
              "performance.actor": [
                {
                  "name@en": "Dylan Russell"
                }
              ]
            }
          ]
        },
        {
          "name@en": "Green Eyes",
          "starring": [
            {
              "performance.actor": [
                {
                  "name@en": "Ignas Miskinis"
                }
              ]
            },
            {
              "performance.actor": [
                {
                  "name@en": "Ieva Matulionytė"
                }
              ]
            }
          ]
        },
        {
          "name@en": "Dogonauts: Enemy Line",
          "starring": [
            {
              "performance.actor": [
                {
                  "name@en": "Don Chatfield"
                }
              ]
            },
            {
              "performance.actor": [
                {
                  "name@en": "Justin Rasch"
                }
              ]
            }
          ]
        },
        {
          "name@en": "Morning Prayers",
          "starring": [
            {
              "performance.actor": [
                {
                  "name@en": "Ismir Gagula"
                }
              ]
            },
            {
              "performance.actor": [
                {
                  "name@en": "Serafedin Redzepov"
                }
              ]
            }
          ]
        },
        {
          "name@en": "A Letter to Uncle Boonmee",
          "starring": [
            {
              "performance.actor": [
                {
                  "name@en": "Nuttapon  Kemthong"
                }
              ]
            },
            {
              "performance.actor": [
                {
                  "name@en": "Kumgieng Jittamaat"
                }
              ]
            }
          ]
        },
        {
          "name@en": "Lot in Sodom",
          "starring": [
            {
              "performance.actor": [
                {
                  "name@en": "Dorthea House"
                }
              ]
            },
            {
              "performance.actor": [
                {
                  "name@en": "Hildegarde Watson"
                }
              ]
            }
          ]
        },
        {
          "name@en": "Rush Hour",
          "starring": [
            {
              "performance.actor": [
                {
                  "name@en": "Christelle Seyvecon"
                }
              ]
            },
            {
              "performance.actor": [
                {
                  "name@en": "Nicolas Guillot"
                }
              ]
            }
          ]
        },
        {
          "name@en": "Sound Collector",
          "starring": [
            {
              "performance.actor": [
                {
                  "name@en": "Steve Alexander"
                }
              ]
            },
            {
              "performance.actor": [
                {
                  "name@en": "Joann McIntyre"
                }
              ]
            }
          ]
        }
      ]
    }
  ]
}
```
Some points to keep in mind while using recurse queries are:

- Each edge would be traversed only once. Hence, cycles would be avoided.
- You can specify only one level of predicates after root. These would be traversed recursively. Both scalar and entity-nodes are treated similarly.
- Only one recurse block is advised per query.
- Be careful as the result size could explode quickly and an error would be returned if the result set gets too large. In such cases use more filter, limit resutls using pagination, or provide a depth parameter at root as follows:

```
curl localhost:8080/query -XPOST -d $'{
	recurse(func: gt(count(~genre), 30000), depth: 2){
		name@en
		~genre (first:2) @filter(gt(count(starring), 2))
		starring (first: 2)
		performance.actor
	}
}'
```

Output:
```
{
  "recurse": [
    {
      "name@en": "Short Film",
      "~genre": [
        {
          "name@en": "Life Begins for Andy Panda"
        },
        {
          "name@en": "Liviu's Dream"
        }
      ]
    },
    {
      "name@en": "Drama",
      "~genre": [
        {
          "name@en": "Prisoners"
        },
        {
          "name@en": "Stoker"
        }
      ]
    },
    {
      "name@en": "Comedy",
      "~genre": [
        {
          "name@en": "A tu per tu"
        },
        {
          "name@en": "Gastone"
        }
      ]
    },
    {
      "name@en": "Documentary film",
      "~genre": [
        {
          "name@en": "Dream Theater: Chaos in Motion"
        },
        {
          "name@en": "Filming Othello"
        }
      ]
    }
  ]
}
```
## Cascade Directive

`@cascade` directive forces a removal of those entites that don't have all the fields specified in the query. This can be useful in cases where some filter was applied. For example, consider this query:

```
curl localhost:8080/query -XPOST -d $'{
  HP(func: allofterms(name, "Harry Potter")) @cascade {
    name@en
    starring{
        performance.character {
          name@en
        }
        performance.actor @filter(allofterms(name, "Warwick")){
            name@en
         }
    }
  }
}'
```
Output:
```
{
  "HP": [
    {
      "name@en": "Harry Potter and the Order of the Phoenix",
      "starring": [
        {
          "performance.actor": [
            {
              "name@en": "Warwick Davis"
            }
          ],
          "performance.character": [
            {
              "name@en": "Professor Filius Flitwick"
            }
          ]
        }
      ]
    },
    {
      "name@en": "Harry Potter and the Philosopher's Stone",
      "starring": [
        {
          "performance.actor": [
            {
              "name@en": "Warwick Davis"
            }
          ],
          "performance.character": [
            {
              "name@en": "Professor Filius Flitwick"
            }
          ]
        },
        {
          "performance.actor": [
            {
              "name@en": "Warwick Davis"
            }
          ],
          "performance.character": [
            {
              "name@en": "Goblin Bank Teller"
            }
          ]
        }
      ]
    },
    {
      "name@en": "Harry Potter and the Prisoner of Azkaban",
      "starring": [
        {
          "performance.actor": [
            {
              "name@en": "Warwick Davis"
            }
          ],
          "performance.character": [
            {
              "name@en": "Professor Filius Flitwick"
            }
          ]
        }
      ]
    },
    {
      "name@en": "Harry Potter and the Half-Blood Prince",
      "starring": [
        {
          "performance.actor": [
            {
              "name@en": "Warwick Davis"
            }
          ],
          "performance.character": [
            {
              "name@en": "Professor Filius Flitwick"
            }
          ]
        }
      ]
    },
    {
      "name@en": "Harry Potter and the Chamber of Secrets",
      "starring": [
        {
          "performance.actor": [
            {
              "name@en": "Warwick Davis"
            }
          ],
          "performance.character": [
            {
              "name@en": "Professor Filius Flitwick"
            }
          ]
        }
      ]
    },
    {
      "name@en": "Harry Potter and the Deathly Hallows – Part 2",
      "starring": [
        {
          "performance.actor": [
            {
              "name@en": "Warwick Davis"
            }
          ],
          "performance.character": [
            {
              "name@en": "Professor Filius Flitwick"
            }
          ]
        },
        {
          "performance.actor": [
            {
              "name@en": "Warwick Davis"
            }
          ],
          "performance.character": [
            {
              "name@en": "Griphook"
            }
          ]
        }
      ]
    },
    {
      "name@en": "Harry Potter and the Deathly Hallows - Part I",
      "starring": [
        {
          "performance.actor": [
            {
              "name@en": "Warwick Davis"
            }
          ],
          "performance.character": [
            {
              "name@en": "Griphook"
            }
          ]
        }
      ]
    },
    {
      "name@en": "Harry Potter and the Goblet of Fire",
      "starring": [
        {
          "performance.actor": [
            {
              "name@en": "Warwick Davis"
            }
          ],
          "performance.character": [
            {
              "name@en": "Professor Filius Flitwick"
            }
          ]
        }
      ]
    }
  ]
}
```

Here we also remove all the nodes that don't have a corresponding valid sibling node for `Warwick Davis`.

## Normalize directive

Queries can have a `@normalize` directive, which if supplied at the root, the response would only contain the predicates which are asked with an alias in the query. The response is also flatter and avoids nesting, hence would be easier to parse in some cases.

```
curl localhost:8080/query -XPOST -d $'{
  director(func:allofterms(name, "steven spielberg")) @normalize {
    d: name@en
    director.film {
      f: name@en
      release_date
      starring {
        performance.actor @filter(anyofterms(name, "tom hanks")) {
          a: name@en
        }
      }
    }
  }
}' | python -m json.tool | less
```


The response for this would be
```
{
  "director": [{
      "d": "Steven Spielberg",
      "f": "Indiana Jones and the Temple of Doom"
    }, {
      "d": "Steven Spielberg",
      "f": "Jaws"
    }, {
      "a": "Tom Hanks",
      "d": "Steven Spielberg",
      "f": "Saving Private Ryan"
    }, {
      "a": "Tom Sizemore",
      "d": "Steven Spielberg",
      "f": "Saving Private Ryan"
    },
    .
    .
    .
  },
  {
    "d": "Young Steven Spielberg"
  },
  {
    "d": "Steven Spielberg"
  }
  ]
}
```
From the results we can see that since we didn't ask for `release_date` with an alias we didn't get it back. We got back all other combinations of movies directed by Steven Spielberg with an actor whose name has anyofterms Tom Hanks.

## Debug

For the purposes of debugging, you can attach a query parameter `debug=true` to a query. Attaching this parameter lets you retrieve the `_uid_` attribute for all the entities along with the `server_latency` information.

Query with debug as a query parameter
```
curl "http://localhost:8080/query?debug=true" -XPOST -d $'{
  director(id: m.07bwr) {
    name@en
  }
}' | python -m json.tool | less
```

Returns `_uid_` and `server_latency`
```
{
  "director": [
    {
      "_uid_": "0xff4c6752867d137d",
      "name@en": "The Big Lebowski"
    }
  ],
  "server_latency": {
    "json": "29.149µs",
    "parsing": "129.713µs",
    "processing": "500.276µs",
    "total": "661.374µs"
  }
}
```

## Fragments

`fragment` keyword allows you to define new fragments that can be referenced in a query, as per [GraphQL specification](https://facebook.github.io/graphql/#sec-Language.Fragments). The point is that if there are multiple parts which query the same set of fields, you can define a fragment and refer to it multiple times instead. Fragments can be nested inside fragments, but no cycles are allowed. Here is one contrived example.

```
curl localhost:8080/query -XPOST -d $'
query {
  debug(id: m.07bwr) {
    name@en
    ...TestFrag
  }
}
fragment TestFrag {
  initial_release_date
  ...TestFragB
}
fragment TestFragB {
  country
}' | python -m json.tool | less
```

## GraphQL Variables

`Variables` can be defined and used in GraphQL queries which helps in query reuse and avoids costly string building in clients at runtime by passing a separate variable map. A variable starts with a $ symbol. For complete information on variables, please check out GraphQL specification on [variables](https://facebook.github.io/graphql/#sec-Language.Variables). We encode the variables as a separate JSON object as show in the example below.

```
curl localhost:8080/query -XPOST -d $'{
 "query": "query test($a: int, $b: int){  me(id: m.06pj8) {director.film (first: $a, offset: $b) {genre(first: $a) { name@en }}}}",
 "variables" : {
  "$a": "5",
  "$b": "10"
 }
}' | python -m json.tool | less
```

* Variables whose type is suffixed with a `!` can't have a default value but must
have a value as part of the variables map.
* The value of the variable must be parsable to the given type, if not, an error is thrown.
* Any variable that is being used must be declared in the named query clause in the beginning.
* We also support default values for the variables. In the example below, `$a` has a
default value of `2`.
```
curl localhost:8080/query -XPOST -d $'{
 "query": "query test($a: int = 2, $b: int!){  me(id: m.06pj8) {director.film (first: $a, offset: $b) {genre(first: $a) { name@en }}}}",
 "variables" : {
   "$a": "5",
   "$b": "10"
 }
}' | python -m json.tool | less
```

* If the variable is initialized in the variable map, the default value will be
overridden (In the example, `$a` will have value 5 and `$b` will be 3).

* The variable types that are supported as of now are: `int`, `float`, `bool` and `string`.


{{% notice "note" %}}In GraphiQL interface, the query and the variables have to be separately entered in their respective boxes.{{% /notice %}}
