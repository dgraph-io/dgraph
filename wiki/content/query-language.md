+++
date = "2017-03-20T18:52:36+11:00"
draft = true
title = "query language"

+++

## GraphQL+- 
Dgraph uses a variation of [GraphQL](https://facebook.github.io/graphql/) as the primary language of communication.
GraphQL is a query language created by Facebook for describing the capabilities and requirements of data models for client‐server applications.
While GraphQL isn't aimed at Graph databases, it's graph-like query syntax, schema validation and subgraph shaped response make it a great language choice.
Having said that, we have modified GraphQL to support graph operations and removed some of the features that we felt weren't a right fit to be a language for a graph database.
We're calling this simplified, feature rich language, ''GraphQL+-''.

{{Note|This language is a work in progress. We're adding more features and we might further simplify some of the existing ones.}}

## Mutations 

{{Note| All mutations queries shown here can be run using `curl localhost:8080/query -XPOST -d $'mutation { ... }'`}}

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

{| class="wikitable"
|-
! Versions up to v0.7.3 !! Versions after v0.7.3 (currently only source builds)
|-
|
Note that Dgraph converts these to `predicate.lang`. So, the above would be equivalent to the following for Dgraph. In fact, this is how we'll be querying for names later.
```
<0x01> <name.ru> "Алисия" .
<0x01> <name.fr> "Adélaïde" .
```
|| 
To specify the language of the value to be returned from query `@lang1:lang2:lang3` notation is used. It is extension over RDF N-Quad syntax, and allows specifying multiple languages in order of preference. If value in given language is not found, next language from list is considered. If there are no values in any of specified languages, the value without specified language is returned. At last, if there is no value without language, value in ''some'' language is returned (this is implementation specific).

{{ Note | Languages preference list cannot be used in functions.}}
|}

### Batch mutations 

You can put multiple RDF lines into a single query to Dgraph. This is highly recommended.
Dgraph loader by default batches a 1000 RDF lines into one query, while running 100 such queries in parallel.

```
mutation {
  set {
    <0x01> <name> "Alice" .
    <0x01> <name> "Алисия"@ru .
    <0x01> <name> "Adélaïde"@fr .
  }
}
```
{{Tip|Using a `$` in front of the curl payload maintains newline characters, which are essential for RDF N-Quads.}}

### Assigning UID 

Blank nodes (`_:<identifier>`) in mutations let you create a new entity in the database by assigning it a UID.
Dgraph can assign multiple UIDs in a single query while preserving relationships between these entities.
Let us say you want to create a database of students of a class using Dgraph, you could create new person entity using this method.
Consider the example below which creates a class, students and some relationships.

```
mutation {
 set {
   _:class <student> _:x .
   _:class <name> "awesome class" .
   _:x <name> "alice" .
   _:x <planet> "Mars" .
   _:x <friend> _:y .
   _:y <name> "bob" .
 }
}

# This on execution returns
{"code":"Success","message":"Done","uids":{"class":13454320254523534561,"x":15823208343267848009,"y":14313500258320550269}}
# Three new entitities have been assigned UIDs.
```

The result of the mutation has a field called `uids` which contains the assigned UIDs.
The mutation above resulted in the formation of three new entities `class`, `x` and `y` which could be used by the user for later reference.
Now we can query as follows using the `_uid_` of `class`.

```
{
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
}

# This query on execution results in
{
    "class": {
        "name": "awesome class",
        "student": {
            "friend": {
                "name": "bob"
            },
            "name": "alice",
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
{{Warning|Dgraph does not store a UID to XID mapping. So, given a UID, it won't locate the corresponding XID automatically. Also, XID collisions are possible and are NOT checked for. If two XIDs fingerprint map to the same UID, their data would get mixed up.}}

You can add and query the above data like so.
```
mutation{
  set {
    <alice> <name> "Alice" .
    <lewis-carrol> <died> "1998" .
  }
}

$ curl localhost:8080/query -XPOST -d '{me(id:alice) {name}}'
```

### Delete 

`delete` keyword lets you delete a specified `S P O` triple through a mutation. For example, if you want to delete the record `<lewis-carrol> <died> "1998" .`, you would do the following.

```
mutation {
  delete {
     <lewis-carrol> <died> "1998" .
  }
}
```

Or you can delete it using the `_uid_` for `lewis-carrol` like

```
mutation {
  delete {
     <0xf11168064b01135b> <died> "1998" .
  }
}
```

## Queries 
{{Note|
Most of the examples here are based on the 21million.rdf.gz file [located here](https://github.com/dgraph-io/benchmarks/blob/master/data/21million.rdf.gz). The geo-location queries are based on sf.tourism.gz file [located here](https://github.com/dgraph-io/benchmarks/blob/master/data/sf.tourism.gz). The corresponding 21million.schema for both is [located here](https://github.com/dgraph-io/benchmarks/blob/master/data/21million.schema).

To try out these queries, you can download these files, run Dgraph and load the data like so.
```
$ dgraph --schema 21million.schema
$ dgraphloader -r 21million.rdf.gz,sf.tourism.gz
```

With Dgraph running, all queries shown can be run using `curl localhost:8080/query -XPOST -d $'{ ... }'`.
}}

Queries in GraphQL+- look very much like queries in GraphQL. You typically start with a node or a list of nodes, and expand edges from there.
Each `{}` block goes one layer deep.

{| class="wikitable"
|-
! Versions up to v0.7.3 !! Versions after v0.7.3 (currently only source builds)
|-
|
```
{
  me(id: m.06pj8) {
    name.en
    director.film  {
      genre {
        name.en
      }
      name.en
      initial_release_date
    }
  }
}
```
||
```
{
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
}
```
|}


```
{
  "me": [
  {
    "director.film": [
      {
        "genre": [
          {
            "name": "Costume Adventure"
          },
          {
            "name": "Adventure Film"
          },
          {
            "name": "Action/Adventure"
          },
          {
            "name": "Action Film"
          }
        ],
        "initial_release_date": "1984-05-22",
        "name": "Indiana Jones and the Temple of Doom"
      },
      ...
      ...
      ...
      {
        "genre": [
          {
            "name": "Drama"
          }
        ],
        "initial_release_date": "1985-12-15",
        "name": "The Color Purple"
      }
    ],
    "name": "Steven Spielberg"
  }
  ]
}
```

What happened above is that we start the query with an entity denoting Steven Spielberg (from Freebase data), then expand by two predicates `name.en` which yields the value `Steven Spielberg`, and `director.film` which yields the entities of films directed by Steven Spielberg.

Then for each of these film entities, we expand by three predicates: `name.en` which yields the name of the film, `initial_release_date` which yields the film release date and `genre` which yields a list of genre entities. Each genre entity is then expanded by `name.en` to get the name of the genre.

If you want to use a list, then the query would look like:

{| class="wikitable"
|-
! Versions up to v0.7.3 !! Versions after v0.7.3 (currently only source builds)
|-
|
```
{
  me(id: [m.06pj8, m.0bxtg]) {
    name.en
    director.film  {
      genre {
        name.en
      }
      name.en
      initial_release_date
    }
  }
}
```
||
```
{
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
```
|}


The list can contain XIDs or UIDs or a combination of both.

## Pagination 
Often there is too much data and you only want a slice of the data.

### First 

If you want just the first few results as you expand the predicate for an entity, you can use the `first` argument. The value of `first` can be positive for the first N results, and ''negative for the last N.''

{{Note|Without a sort order specified, the results are sorted by `_uid_`, which is assigned randomly. So the ordering while deterministic, might not be what you expected.}}

This query retrieves the first two films directed by Steven Spielberg and their last 3 genres.

{| class="wikitable"
|-
! Versions up to v0.7.3 !! Versions after v0.7.3 (currently only source builds)
|-
|
```
{
  me(id: m.06pj8) {
    director.film (first: 2) {
      name.en
      initial_release_date
      genre (first: -3) {
          name.en
      }
    }
  }
 }
```
||
```
{
  me(id: m.06pj8) {
    director.film (first: 2) {
      name@en
      initial_release_date
      genre (first: -3) {
          name@en
      }
    }
  }
 }
```
|}

```
{
    "me": [
        {
            "director.film": [
                {
                    "genre": [
                        {
                            "name": "Adventure Film"
                        },
                        {
                            "name": "Action/Adventure"
                        },
                        {
                            "name": "Action Film"
                        }
                    ],
                    "initial_release_date": "1984-05-23",
                    "name": "Indiana Jones and the Temple of Doom"
                },
                {
                    "genre": [
                        {
                            "name": "Mystery film"
                        },
                        {
                            "name": "Horror"
                        },
                        {
                            "name": "Thriller"
                        }
                    ],
                    "initial_release_date": "1975-06-20",
                    "name": "Jaws"
                }
            ]
        }
    ]
}
```

`first` can also be specified at root.

### Offset 

If you want the '''next''' one result, you want to skip the first two results with `offset:2` and keep only one result  with `first:1`.

{| class="wikitable"
|-
! Versions up to v0.7.3 !! Versions after v0.7.3 (currently only source builds)
|-
|
```
{
  me(id: m.06pj8) {
    name.en
    director.film(first:1, offset:2)  {
      genre {
        name.en
      }
      name.en
      initial_release_date
    }
  }
}
```
||
```
{
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
}
```
|}


Notice the `first` and `offset` arguments. Here is the output which contains only one result.

```
{
  "me": [
    {
      "director.film": [
        {
          "genre": [
            {
              "name": "War film"
            },
            {
              "name": "Drama"
            },
            {
              "name": "Action Film"
            }
          ],
          "initial_release_date": "1998-07-23",
          "name": "Saving Private Ryan"
        }
      ],
      "name": "Steven Spielberg"
    }
  ]
}
```

`offset` can also be specified at root to skip over some results.

### After 

Dgraph assigns `uint64`'s to all entities which are called UIDs (unique internal IDs). All results are sorted by UIDs by default. Therefore, another way to get the next one result after the first two results is to specify that the UIDs of all results are larger than the UID of the second result.

This helps in pagination where the first query would be of the form `<attribute>(after: 0x0, first: N)` and the subsequent ones will be of the form `<attribute>(after: <uid of last entity in last result>, first: N)`

In the above example, the first two results are the film entities of "Indiana Jones and the Temple of Doom" and "Jaws". You can obtain their UIDs by adding the predicate `_uid_` in the query.

{| class="wikitable"
|-
! Versions up to v0.7.3 !! Versions after v0.7.3 (currently only source builds)
|-
|
```
{
  me(id: m.06pj8) {
    name.en
    director.film(first:2) {
      _uid_
      name.en
    }
  }
}
```
||
```
{
  me(id: m.06pj8) {
    name@en
    director.film(first:2) {
      _uid_
      name@en
    }
  }
}
```
|}


The response looks like:
```
{
  "me": [
    {
      "director.film": [
        {
          "_uid_": "0xc17b416e58b32bb",
          "name": "Indiana Jones and the Temple of Doom"
        },
        {
          "_uid_": "0xc6f4b3d7f8cbbad",
          "name": "Jaws"
        }
      ],
      "name": "Steven Spielberg"
    }
  ]
}
```

Now we know the UID of the second result is `0xc6f4b3d7f8cbbad`. We can get the next one result by specifying the `after` argument.

{| class="wikitable"
|-
! Versions up to v0.7.3 !! Versions after v0.7.3 (currently only source builds)
|-
|
```
{
  me(id: m.06pj8) {
    name.en
    director.film(first:1, after:0xc6f4b3d7f8cbbad)  {
      genre {
        name.en
      }
      name.en
      initial_release_date
    }
  }
}
```
||
```
{
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
}
```
|}

The response is the same as before when we use `offset:2` and `first:1`.

```
{
    "me": [
        {
            "director.film": [
                {
                    "genre": [
                        {
                            "name": "War film"
                        },
                        {
                            "name": "Drama"
                        },
                        {
                            "name": "Action Film"
                        }
                    ],
                    "initial_release_date": "1998-07-24",
                    "name": "Saving Private Ryan"
                }
            ],
            "name": "Steven Spielberg"
        }
    ]
}
```

## Alias 

Alias lets us provide alternate names to predicates in results for convenience.

For example, the following query replaces the predicate `name` with `full_name` in the JSON result.
{| class="wikitable"
|-
! Versions up to v0.7.3 !! Versions after v0.7.3 (currently only source builds)
|-
|
```
{
  me(id: m.0bxtg) {
    full_name:name.en
  }
}
```
||
```
{
  me(id: m.0bxtg) {
    full_name:name@en
  }
}
```
|}


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

{| class="wikitable"
|-
! Versions up to v0.7.3 !! Versions after v0.7.3 (currently only source builds)
|-
|
```
{
  me(id: m.0bxtg) {
    name.en
    count(actor.film)
  }
}
```
||
```
{
  me(id: m.0bxtg) {
    name@en
    count(actor.film)
  }
}
```
|}


```
{
 "me": [
  {
   "actor.film": [
     {
       "count": 75
      }
    ],
    "name": "Tom Hanks"
  }
 ]
}
```

## Functions
[Functions](https://dgraph.io) are documented here.

## Filters 

[[#Functions|Functions]] can be applied to results as Filters. 

#### AND, OR and NOT 

Dgraph supports AND, OR and NOT filters. The syntax is of form: `A OR B`, `A AND B`, or `NOT A`. You can add round brackets to make these filters more complex. `(NOT A OR B) AND (C AND NOT (D OR E))`

In this query, we are getting film names which contain either both "indiana" and "jones" OR both "jurassic" AND "park".
{| class="wikitable"
|-
! Versions up to v0.7.3 !! Versions after v0.7.3 (currently only source builds)
|-
|
```
{
  me(id: m.06pj8) {
    name.en
    director.film @filter(allof(name.en, "jones indiana") OR allof(name.en, "jurassic park"))  {
      _uid_
      name.en
    }
  }
}
```
||
```
{
  me(id: m.06pj8) {
    name@en
    director.film @filter(allof(name, "jones indiana") OR allof(name, "jurassic park"))  {
      _uid_
      name@en
    }
  }
}
```
|}


```
{
  "me": [
    {
      "director.film": [
        {
          "_uid_": "0xc17b416e58b32bb",
          "name": "Indiana Jones and the Temple of Doom"
        },
        {
          "_uid_": "0x22e65757df0c94d2",
          "name": "Jurassic Park"
        },
        {
          "_uid_": "0x7d0807a6740c25dc",
          "name": "Indiana Jones and the Kingdom of the Crystal Skull"
        },
        {
          "_uid_": "0x8f2485e4242cbe6e",
          "name": "The Lost World: Jurassic Park"
        },
        {
          "_uid_": "0xa4c4cc65751e98e7",
          "name": "Indiana Jones and the Last Crusade"
        },
        {
          "_uid_": "0xd1c161bed9769cbc",
          "name": "Indiana Jones and the Raiders of the Lost Ark"
        }
      ],
      "name": "Steven Spielberg"
    }
  ]
}
```

#### Filter on Compare 

Dgraph also supports compare filter, which takes form as @filter(compare(count(attribute), N)), only entities fulfill such predication will be returned.

Note: "Compare" here includes "eq", "gt", "geq", "lt", "leg".  And "count" should be applied on non-scalar types.

{| class="wikitable"
|-
! Versions up to v0.7.3 !! Versions after v0.7.3 (currently only source builds)
|-
|
```
{
  director(func:anyof(name.en, "Steven Spielberg")) @filter(gt(count(director.film), 36)) {
    name.en
    count(director.film)
  }
}
```
||
```
{
  director(func:anyof(name, "Steven Spielberg")) @filter(gt(count(director.film), 36)) {
    name@en
    count(director.film)
  }
}
```
|}


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
      "name": "Steven Spielberg"
    },
    {
      "director.film": [
        {
          "count": 115
        }
      ],
      "name": "Steven Scarborough"
    }
  ]
}
```

## Sorting 

We can sort results by a predicate using the `orderasc` or `orderdesc` argument. The predicate has to be indexed and this has to be specified in the schema. As you may expect, `orderasc` sorts in ascending order while `orderdesc` sorts in descending order.

For example, we can sort the films of Steven Spielberg by their release date, in ascending order.

{| class="wikitable"
|-
! Versions up to v0.7.3 !! Versions after v0.7.3 (currently only source builds)
|-
|
```
{
  me(id: m.06pj8) {
    name.en
    director.film(orderasc: initial_release_date) {
      name.en
      initial_release_date
    }
  }
}
```
||
```
{
  me(id: m.06pj8) {
    name@en
    director.film(orderasc: initial_release_date) {
      name@en
      initial_release_date
    }
  }
}
```
|}

```
{
  "me": [
    {
      "director.film": [
        {
          "initial_release_date": "1964-03-23",
          "name": "Firelight"
        },
        {
          "initial_release_date": "1966-12-31",
          "name": "Slipstream"
        },
        {
          "initial_release_date": "1968-12-17",
          "name": "Amblin"
        },
        ...
        ...
        ...
        {
          "initial_release_date": "2012-10-07",
          "name": "Lincoln"
        },
        {
          "initial_release_date": "2015-10-15",
          "name": "Bridge of Spies"
        }
      ],
      "name": "Steven Spielberg"
    }
  ]
}
```

If you use `orderdesc` instead, the films will be listed in descending order.

{| class="wikitable"
|-
! Versions up to v0.7.3 !! Versions after v0.7.3 (currently only source builds)
|-
|
```
{
  me(id: m.06pj8) {
    name.en
    director.film(orderdesc: initial_release_date, first: 2) {
      name.en
      initial_release_date
    }
  }
}
```
||
```
{
  me(id: m.06pj8) {
    name@en
    director.film(orderdesc: initial_release_date, first: 2) {
      name@en
      initial_release_date
    }
  }
}
```
|}


Here is the output.

```
{
  "me": [
    {
      "director.film": [
        {
          "initial_release_date": "2015-10-15",
          "name": "Bridge of Spies"
        },
        {
          "initial_release_date": "2012-10-07",
          "name": "Lincoln"
        },
      ],
      "name": "Steven Spielberg"
    }
  ]
}
```

To sort at root level, we can do as follows:
{| class="wikitable"
|-
! Versions up to v0.7.3 !! Versions after v0.7.3 (currently only source builds)
|-
|
```
{
  me(func: allof(name.en, "steven"), orderasc: name.en) {
    name.en
  }
}
```
||
```
{
  me(func: allof(name, "ste"), orderasc: name) {
    name@en
  }
}
```
|}

```
{
    "me": [
        {
            "name": "Steven's Friend #1"
        },
        {
            "name": "Steven's Friend #2"
        },
        {
            "name": "Steven's Orderly #2"
        },
        {
            "name": "United Standards 2 (as Steven Carell)"
        },
        {
            "name": "American Boy: A Profile of Steven Prince"
        },
        {
            "name": "Steven A Burd"
....
 {
            "name": "Steven Kirshoff"
        },
        {
            "name": "Steven Kirtley"
        },
        {
            "name": "Steven Kissel"
        }
    ]
}
```


##Facets : Edge attributes

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
mutation { 
 set {
  <alice> <name> "alice" .
  <alice> <mobile> "040123456" (since=2006-01-02T15:04:05) .
  <alice> <car> "MA0123" (since=2006-02-02T13:01:09, first=true) .
 }
}
```

Querying `name` and `mobile` of `alice` gives us :
```
{
  data(id:alice) {
     name
     mobile
     car
  }
}
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
{
  data(id:alice) {
     name
     mobile @facets(since)
     car @facets(since)
  }
}
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
{
  data(id:alice) {
     name
     mobile @facets
     car @facets
  }
}

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

###Facets on Uid edges

`friend` is an edge with facet `close`.
It is set to true for friendship between alice and bob
and false for friendship between alice and charlie.

```
mutation { 
 set {
  <alice> <name> "alice" .
  <bob> <name> "bob" .
  <bob> <car> "MA0134" (since=2006-02-02T13:01:09) .
  <charlie> <name> "charlie" .
  <alice> <friend> <bob> (close=true) .
  <alice> <friend> <charlie> (close=false) .
 }
}
```

If we query friends of `alice` we get :
```
{
   data(id:alice) {
     name
     friend {
       name
     }
   }
}
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
{
   data(id:alice) {
     name
     friend @facets(close) {
       name
     }
   }
}
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
{
   data(id:alice) {
     name 
     friend @facets {
       name
       car @facets
     }
   }
}
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

We are working on supporting filtering, sorting and pagination based on facets in the query.

##Aggregation
Aggregation is process in which information is gathered and expressed in a summary form. A common aggregation purpose is to get more information about particular groups based on specific variables such as age, profession, or income.

Note: we support aggregation on scalar type only.

#### Min 
```
{
  director(id: m.06pj8) {
    director.film {
    	min(initial_release_date)
    }
  }
}
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

#### Max 
```
{
  director(id: m.06pj8) {
    director.film {
    	max(initial_release_date)
    }
  }
}
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

#### Sum 
```
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
		}
	}
}
```

Output:

```
{
  "me": [
    {
      "friend": [
        {
          "sum(age)": 300
        },
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
        }
      ]
    }
  ]
}
```

## Multiple Query Blocks 
Multiple blocks can be inside a single query and they would be returned in the result with the corresponding block names.

{| class="wikitable"
|-
! Versions up to v0.7.3 !! Versions after v0.7.3 (currently only source builds)
|-
|
```
{
 AngelinaInfo(func:allof(name.en, "angelina jolie")) {
  name.en
   actor.film {
    performance.film {
      genre {
        name.en
      }
    }
   }
  }

 DirectorInfo(id: m.06pj8) {
    name.en
    director.film @filter(geq("initial_release_date", "2008"))  {
        Release_date: initial_release_date
        Name: name.en
    }
  }
}
```
||
```
{
 AngelinaInfo(func:allof(name, "angelina jolie")) {
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
    director.film @filter(geq("initial_release_date", "2008"))  {
        Release_date: initial_release_date
        Name: name@en
    }
  }
}
```
|}

```
{
  "AngelinaInfo": [
    {
      "name": "Angelina Jolie Look-a-Like"
    },
    {
      "name": "Angelina Jolie"
    },
    {
      "actor.film": [
        {
          "performance.film": [
            {
              "genre": [
                {
                  "name": "Animation"
                },
                {
                  "name": "Fantasy"
                },
                {
                  "name": "Computer Animation"
                },
                {
                  "name": "Adventure Film"
                },
                {
                  "name": "Action Film"
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
                  "name": "Comedy"
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
      "name": "Steven Spielberg"
    }
  ]
}
```

###Var Blocks
Var blocks are the blocks which start with the keyword `var` and these blocks are not returned in the query results.

{| class="wikitable"
|-
! Versions up to v0.7.3 !! Versions after v0.7.3 (currently only source builds)
|-
|
```
{
 var(func:allof(name.en, "angelina jolie")) {
  name.en
   actor.film {
    A AS performance.film {
     B AS genre
    }
   }
  }

 films(var:B) {
   ~genre @filter(id(A)) {
     name.en
   }
 }
}
```
||
```
{
 var(func:allof(name, "angelina jolie")) {
  name@en
   actor.film {
    A AS performance.film {
     B AS genre
    }
   }
  }

 films(var:B) {
   ~genre @filter(id(A)) {
     name@en
   }
 }
}
```
|}

This query returns.
```
{
  "films": [
    {
      "~genre": [
        {
          "name": "Hackers"
        }
      ]
    },
    {
      "~genre": [
        {
          "name": "Mr. & Mrs. Smith"
        }
      ]
    },
    {
      "~genre": [
        {
          "name": "Hackers"
        },
        {
          "name": "Foxfire"
        }
      ]
    },
.
.
.
      {
          "name": "Without Evidence"
        },
        {
          "name": "True Women"
        },
        {
          "name": "Salt"
        }
      ]
    },
    {
      "~genre": [
        {
          "name": "Confessions of an Action Star"
        }
      ]
    },
    {
      "~genre": [
        {
          "name": "Without Evidence"
        }
      ]
    }
  ]
}
```

## Query Variables 

Variables can be defined at different levels of the query using the keyword `AS` and would contain the result list at that level as its value. These variables can be used in other query blocks or the same block (as long as it is used in the child nodes).

For example:
{| class="wikitable"
|-
! Versions up to v0.7.3 !! Versions after v0.7.3 (currently only source builds)
|-
|
```
{
 var(func:allof(name.en, "angelina jolie")) {
  name.en
   actor.film {
    A AS performance.film {  # All the films done by Angelina Jolie
     B As genre  # Genres of all the films done by Angelina Jolie
    }
   }
  }

 var(func:allof(name.en, "brad pitt")) {
  name.en
   actor.film {
    C AS performance.film {  # All the films done by Brad Pitt
     D as genre  # Genres of all the films done by Brad Pitt
    }
   }
  }

 films(var:D) @filter(id(B)) {   # movies done by both Angelina and Brad
  name.en
   ~genre @filter(id(A) OR id(C)) {  # Genres of movies done by Angelina or Brad
     name.en
   }
 }
}
```
||
```
{
 var(func:allof(name, "angelina jolie")) {
  name@en
   actor.film {
    A AS performance.film {  # All the films done by Angelina Jolie
     B As genre  # Genres of all the films done by Angelina Jolie
    }
   }
  }

 var(func:allof(name, "brad pitt")) {
  name@en
   actor.film {
    C AS performance.film {  # All the films done by Brad Pitt
     D as genre  # Genres of all the films done by Brad Pitt
    }
   }
  }

 films(var:D) @filter(id(B)) {   # movies done by both Angelina and Brad
  name@en
   ~genre @filter(id(A) OR id(C)) {  # Genres of movies done by Angelina or Brad
     name@en
   }
 }
}
```
|}


```
{
  "films": [
    {
      "~genre": [
        {
          "name": "Hackers"
        },
        {
          "name": "No Way Out"
        }
      ]
    },
    {
      "~genre": [
        {
          "name": "Being John Malkovich"
        },
        {
          "name": "Burn After Reading"
        },
        {
          "name": "Inglourious Basterds"
        },
        {
          "name": "Mr. & Mrs. Smith"
        }
      ]
    },
    {
      "~genre": [
        {
          "name": "Less Than Zero"
        },
        {
          "name": "Hackers"
        },
        {
          "name": "Foxfire"
.
.
.
     "name": "Salt"
        }
      ]
    },
    {
      "~genre": [
        {
          "name": "Johnny Suede"
        },
        {
          "name": "Confessions of an Action Star"
        }
      ]
    },
    {
      "~genre": [
        {
          "name": "Without Evidence"
        },
        {
          "name": "Too Young to Die?"
        }
      ]
    }
  ]
}

```

## Shortest Path Queries 

Shortest path between a `src` node and `dst` node can be found using the keyword `shortest` for the query block name. It requires the source node id, destination node id and the predicates (atleast one) that have to be considered for traversing. This query block by itself will not return any results back but the path has to be stored in a variable and used in other query blocks as required.

{{ Note | If no predicates are specified in the `shortest` block, no path can be fetched as no edge is traversed. }}

For example:
```
# Insert this via mutation
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
}
```
```
{
 path as shortest(from:a, to:d) {
  friend
 }
 path(var: path) {
   name
 }
}
```
Would return the following results. (Note that each edges' weight is considered as 1)
 ```
{
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
{{ Note | We can specify exactly one facet per predicate in the shortest query block }}
```
{
 path as shortest(from:a, to:d) {
  friend @facets(weight)
 }
 
 path(var: path) {
  name
 }
}
```
```
{
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
{| class="wikitable"
|-
! Versions up to v0.7.3 !! Versions after v0.7.3 (currently only source builds)
|-
|
```
{
  path as shortest(from: 1000, to: 100) {
    friend @filter(not anyof(name.en, "alice")) @facets(weight)
    relative @facets(liking)
  }

  relationship(var: path) {
    name
  }
}
```
||
```
{
  path as shortest(from: 1000, to: 100) {
    friend @filter(not anyof(name, "alice")) @facets(weight)
    relative @facets(liking)
  }

  relationship(var: path) {
    name
  }
}
```
|}

This query would again retrieve the shortest path but using some different parameters for the edge weights which are specified using facets (weight and liking). Also, we'd not like to have any person whose name contains `alice` in the path which is specified by the filter.

## Normalize directive 

{{ Note | This feature will be part of the next release. If you wan't to try it out you can build dgraph from the master branch. }}

Queries can have a `@normalize` directive, which if supplied at the root, the response would only contain the predicates which are asked with an alias in the query. The response is also flatter and avoids nesting, hence would be easier to parse in some cases.

{| class="wikitable"
|-
! Versions up to v0.7.3 !! Versions after v0.7.3 (currently only source builds)
|-
|
```
{
  director(func:allof("name.en", "steven spielberg")) @normalize {
    d: name.en
    director.film {
      f: name.en
      release_date
      starring {
        performance.actor @filter(anyof(name.en, "tom hanks")) {
	  a: name.en
	}
      }
    }
  }
}
```
||
```
{
  director(func:allof("name", "steven spielberg")) @normalize {
    d: name@en
    director.film {
      f: name@en
      release_date
      starring {
        performance.actor @filter(anyof(name, "tom hanks")) {
	  a: name@en
	}
      }
    }
  }
}
```
|}


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
From the results we can see that since we didn't ask for `release_date` with an alias we didn't get it back. We got back all other combinations of movies directed by Steven Spielberg with an actor whose name has anyof Tom Hanks.

## Schema 
[Schema](https://dgraph.io) is located here.

## RDF Types 
RDF types can also be used to specify the type of values. They can be attached to the values using the `^^` separator.

```
mutation {
 set {
  _:a <name> "Alice" .
  _:a <age> "15"^^<xs:int> .
  _:a <health> "99.99"^^<xs:float> .   
 }
}
```
This implies that name be stored as string(default), age as int and health as float.

{{Note| RDF type overwrites [[#Schema|schema type]] in case both are present. If both the RDF type and the schema type is missing, value is assumed to be of default type which is stored as string internally.}}

### Supported 

The following table lists all the supported [RDF datatypes](https://www.w3.org/TR/rdf11-concepts/#section-Datatypes) and the corresponding internal type format in which the data is stored.

| Storage Type | Dgraph type |
| -------------|:------------:|
|  <xs:string> | String |
|  <xs:dateTime> |                               DateTime |
|  <xs:date> |                                   Date |
|  <xs:int> |                                    Int32 |
|  <xs:boolean> |                                Bool |
|  <xs:double> |                                 Float |
|  <xs:float> |                                  Float |
|  <geo:geojson> |                               Geo |
|  <http://www.w3.org/2001/XMLSchema#string> |   String |
|  <http://www.w3.org/2001/XMLSchema#dateTime> | DateTime |
|  <http://www.w3.org/2001/XMLSchema#date> |     Date |
|  <http://www.w3.org/2001/XMLSchema#int> |      Int32 |
|  <http://www.w3.org/2001/XMLSchema#boolean> |  Bool |
|  <http://www.w3.org/2001/XMLSchema#double> |   Float |
|  <http://www.w3.org/2001/XMLSchema#float> |    Float |


In case a predicate has different schema type and storage type, the convertibility between the two is ensured during mutation and an error is thrown if they are incompatible.  The values are always stored as storage type if specified, or else they are converted to schema type and stored.

### Extended 

We also support extended types besides RDF types. 

#### Password type 

Password for an entity can be set using the password type, and those attributes can not be queried directly but only be checked for a match using a function which is used to do the password verification.

To set password for a given node you need to add `^^<pwd:password>`, as below
```
mutation {
 set {
	<m.06pj8> <password> "JailBreakers"^^<pwd:password>     .
 }
}
```

And later to check if some value is a correct password, you'd use a `checkpwd` function which would return a boolean value, true meaning the value was right and flase otherwise.
{| class="wikitable"
|-
! Versions up to v0.7.3 !! Versions after v0.7.3 (currently only source builds)
|-
|
```
{
  director(id:m.06pj8) {
    name.en
    checkpwd("JailBreakers")
  }
}
```
||
```
{
  director(id:m.06pj8) {
    name@en
    checkpwd("JailBreakers")
  }
}
```
|}


Output:
```
{
  "director": [
    {
      "name": "Steven Spielberg",
      "password": [
        {
          "checkpwd": true
        }
      ]
    }
  ]
}
```

## Debug 

For the purposes of debugging, you can attach a query parameter `debug=true` to a query. Attaching this parameter lets you retrieve the `_uid_` attribute for all the entities along with the `server_latency` information.

Query with debug as a query parameter
{| class="wikitable"
|-
! Versions up to v0.7.3 !! Versions after v0.7.3 (currently only source builds)
|-
|
```
curl "http://localhost:8080/query?debug=true" -XPOST -d $'{
  director(id: m.07bwr) {
    name.en
  }
}'
```
||
```
curl "http://localhost:8080/query?debug=true" -XPOST -d $'{
  director(id: m.07bwr) {
    name@en
  }
}'
```
|}

Returns `_uid_` and `server_latency`
```
{
  "director": [
    {
      "_uid_": "0xff4c6752867d137d",
      "name": "The Big Lebowski"
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
query {
  debug(id: m.07bwr) {
    name.en
    ...TestFrag
  }
}
fragment TestFrag {
  initial_release_date
  ...TestFragB
}
fragment TestFragB {
  country
}
```

## GraphQL Variables 

`Variables` can be defined and used in GraphQL queries which helps in query reuse and avoids costly string building in clients at runtime by passing a separate variable map. A variable starts with a $ symbol. For complete information on variables, please check out GraphQL specification on [variables](https://facebook.github.io/graphql/#sec-Language.Variables). We encode the variables as a separate JSON object as show in the example below.

```
{
 "query": "query test($a: int, $b: int){  me(id: m.06pj8) {director.film (first: $a, offset: $b) {genre(first: $a) { name.en }}}}",
 "variables" : {
  "$a": "5",
  "$b": "10"
 }
}
```

The type of a variable can be suffixed with a ! to enforce that the variable must have a value. Also, the value of the variable must be parsable to the given type, if not, an error is thrown. Any variable that is being used must be declared in the named query clause in the beginning. And we also support default values for the variables. Example:

```
{
 "query": "query test($a: int = 2, $b: int! = 3){  me(id: m.06pj8) {director.film (first: $a, offset: $b) {genre(first: $a) { name.en }}}}",
 "variables" : {
  "$a": "5"
 }
}
```

If the variable is initialized in the variable map, the default value will be overridden (In the example, $a will be 5 and $b will be 3).

The variable types that are supported as of now are: `int`, `float`, `bool` and `string`.

{{Note| In GraphiQL interface, the query and the variables have to be separately entered in their respective boxes.}}

