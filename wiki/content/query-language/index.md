+++
title = "Query Language - GraphQL+-"
+++

Dgraph's GraphQL+- is based on Facebook's [GraphQL](https://facebook.github.io/graphql/).  GraphQL wasn't developed for Graph databases, but it's graph-like query syntax, schema validation and subgraph shaped response make it a great language choice.  We've modified the language to better support graph operations, adding and removing features to get the best fit for graph databases.  We're calling this simplified, feature rich language, ''GraphQL+-''.

{{% notice "note" %}}GraphQL+- is a work in progress. We're adding more features and we might further simplify existing ones.{{% /notice %}}

## Take a Tour - https://tour.dgraph.io

This document is the Dgraph query reference material.  It is not a tutorial.  It's designed as a reference for users who already know how to write queries in GraohQL+- but need to check syntax, or indices, or functions, etc.

If you are new to Dgraph and want to learn how to use Dgraph and GraphQL+-, take the tour - https://tour.dgraph.io

### Running examples

The examples in this reference use a database of 21 million triples about movies and actors.  The example queries run and return results.  The queries are executed by an instance of Dgraph running at https://play.dgraph.io/.  To run the queries locally or experiment a bit more, see the Getting Started guide on how to start Dgraph and how to load the 21million and tourism (for location queries) datasets.

## Base Query Syntax

**TODO ... grammar in here??? with basic query evaluation description**


### Language Support

Dgraph supports UTF-8 strings.  

For a string valued edge `edge`, the syntax
```
edge@lang1:...:langN
```
specifies the preference order for returned languages with the following rules:

* at most one result will be returned
* if results exists in the preferred languages, the left most (in the preference list) of these is returned
* if no result exists in the preferred languages
  - a result without a language tag is returned, if it exists, or
  - some language tagged result is returned if one exists, and no untagged result exists, or
  - the edge has no matching result otherwise.

In functions, a preference list is not allowed.  A string edge without a language tag means apply function to all languages, while a single language tag means apply to only the given language.


For example, some of Bollywood actor Farhan Akhtar's movies have a name stored in Russian as well as Hindi and English, others do not.

{{< runnable >}}{
  q(func: allofterms(name, "Farhan Akhtar")) {
    name@hi
    name@en

    director.film {
      name@ru:hi:en
      name@en
      name@hi
      name@ru
    }
  }
}{{< /runnable >}}


## Functions

{{% notice "note" %}}Functions can only be applied to [indexed predicates]({{< relref "#indexing">}}).{{% /notice %}}

Functions allow filtering based on properties of nodes or variables.

grammar **TODO placeholder**
...but then some functions allow variables and some don't, better to stop grammar at this point and just do per function
```
fnName(var | edge, value)
value -> "string literal" | /regular expression/ | numeric
regular expression -> go regular expressions https://golang.org/pkg/regexp/syntax/ ???
```

...more here about how they work in general .. and distinction between at root and in filter

note about functions and Language


### Term matching

#### AllOfTerms

Syntax : `allofterms(predicate, "space-separated term list")`

Schema types : `string`

Index required : `term`


Matches strings that have all specified terms in any order; case insensitive.

##### Usage at root

Query Example : All nodes that have `name` containing terms `indiana` and `jones`, returning the english name and genre in english.

{{< runnable >}}
{
  me(func: allofterms(name, "jones indiana")) {
    name@en
    genre {
      name@en
    }
  }
}
{{< /runnable >}}

##### Usage as Filter

Query Example : Steven Spielberg is XID `m.06pj8`.  All his films that contain the words `indiana` and `jones`.

{{< runnable >}}
{
  me(id: m.06pj8) {
    name@en
    director.film @filter(allofterms(name@en, "jones indiana"))  {
      name@en
    }
  }
}
{{< /runnable >}}


#### AnyOfTerms


Syntax : `anyofterms(predicate, "space-separated term list")`

Schema types : `string`

Index required : `term`


Matches strings that have any of the specified terms in any order; case insensitive.

##### Usage at root

Query Example : All nodes that have a `name` containing either `purple` or `peacock`.  Many of the returned nodes are movies, but people like Joan Peacock also meet the search terms because without a [cascade directive]({{< relref "#cascade-directive">}}) the query doesn't require a genre.

{{< runnable >}}
{
  me(func:anyofterms(name@en, "poison peacock")) {
    name@en
    genre {
      name@en
    }
  }
}
{{< /runnable >}}


##### Usage as filter

Query Example : Steven Spielberg is XID `m.06pj8`.  All his movies that contain `war` or `spies`

{{< runnable >}}
{
  me(id: m.06pj8) {
    name@en
    director.film @filter(anyofterms(name, "war spies"))  {
      name@en
    }
  }
}
{{< /runnable >}}


### Regular Expressions


Syntax : `regexp(predicate, /regular-expression/)` or case insensitive `regexp(predicate, /regular-expression/i)`

Schema types : `string`

Index required : `trigram`


Matches strings by regular expression.  The regular expression language is that of [go regular expressions](https://golang.org/pkg/regexp/syntax/).

Query Example : At root, match nodes with `Steven Sp` at the start of `name`, followed by any characters.  For each such matched uid, match the films containing `ryan`.  Note the difference with `allofterms`, which would match only `ryan` but regular expression search will also match within terms, such as `bryan`.

{{< runnable >}}
{
  directors(func: regexp(name@en, /^Steven Sp.*$/)) {
    name@en
    director.film @filter(regexp(name@en, /ryan/i)) {
      name@en
    }
  }
}
{{< /runnable >}}


#### Technical details

A Trigram is a substring of three continuous runes. For example, `Dgraph` has trigrams `Dgr`, `gra`, `rap`, `aph`.

To ensure efficiency of regular expression matching, Dgraph uses [trigram indexing](https://swtch.com/~rsc/regexp/regexp4.html).  That is, Dgraph converts the regular expression to a trigram query, uses the trigram index and trigram query to find possible matches and applies the full regular expression search only to the possibles.

#### Writing Efficient Regular Expressions and Limitations

Keep the following in mind when designing regular expression queries.

- At least one trigram must be matched by the regular expression (patterns shorter than 3 runes are not supported).  That is, Dgraph requires regular expressions that can be converted to a trigram query.
- The number of alternative trigrams matched by the regular expression should be as small as possible  (`[a-zA-Z][a-zA-Z][0-9]` is not a good idea).  Many possible matches means the full regular expression is checked against many strings; where as, if the expression enforces more trigrams to match, Dgraph can make better use of the index and check the full regular expression against a smaller set of possible matches.
- Thus, the regular expression should be as precise as possible.  Matching longer strings means more required trigrams, which helps to effectively use the index.
- If repeat specifications (`*`, `+`, `?`, `{n,m}`) are used, the entire regular expression must not match the _empty_ string or _any_ string: for example, `*` may be used like `[Aa]bcd*` but not like `(abcd)*` or `(abcd)|((defg)*)`
- Repeat specifications after bracket expressions (e.g. `[fgh]{7}`, `[0-9]+` or `[a-z]{3,5}`) are often considered as matching any string because they match too many trigrams.
- If the partial result (for subset of trigrams) exceeds 1000000 uids during index scan, the query is stopped to prohibit expensive queries.


### Full Text Search

Syntax : `alloftext(predicate, "space-separated text")` and `anyoftext(predicate, "space-separated text")`

Schema types : `string`

Index required : `fulltext`


Apply full text search with stemming and stop words to find strings matching all or any of the given text.  

The following steps are applied during index generation and to process full text search arguments:

1. Tokenization (according to Unicode word boundaries).
1. Conversion to lowercase.
1. Unicode-normalization (to [Normalization Form KC](http://unicode.org/reports/tr15/#Norm_Forms)).
1. Stemming using language-specific stemmer.
1. Stop words removal (language-specific).

Following table contains all supported languages and corresponding country-codes.

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


Query Example : All names that have `run`, `running`, etc and `man`.  Stop word removal eliminates `the` and `maybe`

{{< runnable >}}
{
  movie(func:alloftext(name@en, "the man maybe runs")) {
	 name@en
  }
}
{{< /runnable >}}


### Inequality

#### equal to

Syntax :

* `eq(predicate, value)`
* `eq(var(varName), value)`
* `eq(count(predicate), value)`

Schema types : `int`, `float`, `string`, `dateTime`

Index required : An index is required for `eq(predicate, value)` form, but otherwise the values have been calculated as part of the query, so no index is required.

| Type       | Index Options |
|:-----------|:--------------|
| `int`      | `int`         |
| `float`    | `float`       |
| `string`   | `exact`, `hash`, `term`, `fulltext` |
| `dateTime` | `dateTime`    |

Note that `eq(count(predicate), value)` iterates through the every `s predicate o` tripple to first generate the count and then applies the filter to the generated counts.  If there are many such triples, this many not be efficient.

Query Example : Movies with exactly two genres.

{{< runnable >}}
{
  me(func: eq(count(genre), 2)) {
    name@en
  }
}
{{< /runnable >}}


#### Less than, less than or equal to, greater than and greater than or equal to

Syntax : for inequality `IE`

* `IE(predicate, value)`
* `IE(var(varName), value)`
* `IE(count(predicate), value)`

With `IE` replaced by

* `le` less than or equal to
* `lt` less than
* `ge` greater than or equal to
* `gt` greather than

Schema types : `int`, `float`, `string`, `dateTime`

Index required : An index is required for the `IE(predicate, value)` form, but otherwise the values have been calculated as part of the query, so no index is required.

| Type       | Index Options |
|:-----------|:--------------|
| `int`      | `int`         |
| `float`    | `float`       |
| `string`   | `exact`       |
| `dateTime` | `dateTime`    |


Query Example : Steven Spielberg is XID `m.06pj8`.  All his movies released before 1970.

{{< runnable >}}
{
  me(id: m.06pj8) {
    name@en
    director.film @filter(lt(initial_release_date, "1970-01-01"))  {
      initial_release_date
      name@en
    }
  }
}
{{< /runnable >}}


Query Example : Movies with directors with `Steven` in `name` and have directed more than `100` actors.

{{< runnable >}}
{
  ID as var(func: allofterms(name@en, "Steven")) {
    director.film {
      num_actors as count(starring)
    }
    total as sum(var(num_actors))
  }

  dirs(id: var(ID)) @filter(gt(var(total), 100)) {
    name@en
    total_actors : var(total)
  }
}
{{< /runnable >}}


Query Example : All directors.  Count in the root filter can help in determining classes of nodes.  Directors have directed at least one film.  


{{< runnable >}}
{
	all_directors(func: gt(count(director.film), 0)){
		name@en
	}
}
{{< /runnable >}}


Query Example : A movie in each genre that has over 30000 movies.

{{< runnable >}}
{
	genre(func: gt(count(~genre), 30000)){
		name@en
		~genre (first:1) {
			name@en
		}
	}
}
{{< /runnable >}}


### Geolocation

Note that for geo queries, any polygon with holes is replace with the outer loop, ignoring holes.  Also, as for version 0.7.7 polygon containment checks are approximate.

#### Near

Syntax : `near(predicate, [long, lat], distance)`

Schema types : `geo`

Index required : `geo`

Matches all entities where the location given by `predicate` is within `distance` metres of geojson coordinate `[long, lat]`.

Query Example : Tourist destinations within 1 kilometer of a point in Golden Gate Park, San Fransico.

{{< runnable >}}
{
  tourist(func: near(loc, [-122.469829, 37.771935], 1000) ) {
    name
  }
}
{{< /runnable >}}


#### Within

Syntax : `within(predicate, [[long1, lat1], ..., [longN, latN]])`

Schema types : `geo`

Index required : `geo`

Matches all entities where the location given by `predicate` lies within the polygon specified by the geojson coordinate array.

Query Example : Tourist destinations within the specified area of Golden Gate Park, San Fransico.  

{{< runnable >}}
{
  tourist(func: within(loc, [[-122.47266769409178, 37.769018558337926 ], [ -122.47266769409178, 37.773699921075135 ], [ -122.4651575088501, 37.773699921075135 ], [ -122.4651575088501, 37.769018558337926 ], [ -122.47266769409178, 37.769018558337926]] )) {
    name
  }
}
{{< /runnable >}}


#### Contains

Syntax : `contains(predicate, [long, lat])` or `contains(predicate, [[long1, lat1], ..., [longN, latN]])`

Schema types : `geo`

Index required : `geo`

Matches all entities where the polygon describing the location given by `predicate` contains geojson coordinate `[long, lat]` or given geojson polygon.

Query Example : All entities that contain a point in the flamingo enclosure of San Fransico Zoo.
{{< runnable >}}
{
  tourist(func: contains(loc, [ -122.50326097011566, 37.73353615592843 ] )) {
    name
  }
}
{{< /runnable >}}


#### Intersects

Syntax : `intersects(predicate, [[long1, lat1], ..., [longN, latN]])`

Schema types : `geo`

Index required : `geo`

Matches all entities where the polygon describing the location given by `predicate` intersects the given geojson polygon.


{{< runnable >}}
{
  tourist(func: intersects(loc, [[-122.503325343132, 37.73345766902749 ], [ -122.503325343132, 37.733903134117966 ], [ -122.50271648168564, 37.733903134117966 ], [ -122.50271648168564, 37.73345766902749 ], [ -122.503325343132, 37.73345766902749]] )) {
    name
  }
}
{{< /runnable >}}



## Filters


**TODO grammar placeholder**

filter is a function or a filter joined with a boolean connective
 `filter := function | filter AND filter | filter OR filter | NOT filter | (filter)`

usage at root
`q(id: UID | XID) [@filter(filter)]`
or
`q(func: function) [@filter(filter)]`

usage inside query block
`edge @filter(filter) { ... }`


### AND, OR and NOT

Connectives `AND`, `OR` and `NOT` join filters and can be built into arbitrarily complex filters, such as `(NOT A OR B) AND (C AND NOT (D OR E))`.  Note that, `NOT` binds more tightly than `AND` which binds more tightly than `OR`.

Query Example : Steven Spielberg is XID `m.06pj8`.  All his movies that contain either both "indiana" and "jones" OR both "jurassic" and "park".

{{< runnable >}}
{
  me(id: m.06pj8) {
    name@en
    director.film @filter(allofterms(name, "jones indiana") OR allofterms(name, "jurassic park"))  {
      _uid_
      name@en
    }
  }
}
{{< /runnable >}}




















## Schema

For each predicate, the schema specifies the target's type.  If a predicate `p` has type `T`, then for all triples `s p o` the object `o` is of schema type `T`.

* On mutations, scalar types are checked and an error thrown if the value cannot be converted to the schema type.

* On query, value results are returned according to the schema type of the predicate. **Does this mean in the protobuf? cause the JSOn is untyped ... or that the JSON type is parsable into a var of the schema type**??

If a schema type isn't specified before a mutation adds triples for a predicate, then the type is inferred from the first mutation.  This type is either:

* type `uid`, if the first mutation for the predicate has nodes for the subject and object, or

* derived from the [rdf type]({{< relref "#rdf-types" >}}), if the subject is a literal and an rdf type is present in the first mutation, or

* `default` type, otherwise.


### Schema Types

Dgraph supports scalar types and the UID type.

#### Scalar Types

For all scalar types the object is a literal.

| Dgraph Type | Go type |
| ------------|:--------|
|  `default`  | string  |
|  `int`      | int64   |
|  `float`    | float   |
|  `string`   | string  |
|  `bool`     | bool    |
|  `id`       | string  |
|  `dateTime` | time.Time (RFC3339 format [Optional timezone] eg: 2006-01-02T15:04:05.999999999+10:00 or 2006-01-02T15:04:05.999999999)    |
|  `geo`      | [go-geom](https://github.com/twpayne/go-geom)    |

#### UID Type

The `uid` type denotes a node-node edge; internally each node is represented as a `uint64` id.

| Dgraph Type | Go type |
| ------------|:--------|
|  `uid`      | uint64  |


### RDF Types

RDF types are attached to literals with the standard `^^` separator.

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
| &#60;http&#58;//www.w3.org/2001/XMLSchema#string&#62;   | `string`         |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#dateTime&#62; | `dateTime`       |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#date&#62;     | `dateTime`       |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#int&#62;      | `int`            |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#boolean&#62;  | `bool`           |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#double&#62;   | `float`          |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#float&#62;    | `float`          |

As well as implying a schema type for a [first mutation]({{< relref "#schema" >}}), an RDF type can override a schema type for storage.

If a predicate has a schema type and a mutation has an RDF type with a different underlying Dgraph type, the convertibility to schema type is checked, and an error is thrown if they are incompatible, but the value is stored in the RDF type's corresponding Dgraph type.


For example, if no schema is set for the `age` predicate.  Given the mutation
```
mutation {
 set {
  _:a <age> "15"^^<xs:int> .
  _:b <age> "13" .
  _:c <age> "14"^^<xs:string> .
  _:d <age> "14.5"^^<xs:string> .
  _:e <age> "14.5"
 }
}
```
Dgraph:

* sets the schema type to `int`, as implied the first triple,  
* converts `"13"` to `int` on storage,
* checks `"14"` can be converted to `int`, but stores as `string`,
* throws an error for the remaining two triples, because `"14.5"` can't be converted to `int`.

### Extended Types

The following types are also accepted.

#### Password type

A password for an entity is set with `^^<pwd:password>`.  Passwords cannot be queried directly, only checked for a match using the `checkpwd` function.

For example: to set a password:
```
mutation {
  set {
    <ex123> <name> "Password Example"
    <ex123> <password> "ThePassword"^^<pwd:password>     .
  }
}
```

to check a password:
```
{
  check(id: ex123) {
    name
    checkpwd(password, "ThePassword")
  }
}
```

output:
```
{
  "check": [
    {
      "name": "Password Example",
      "password": [
        {
          "checkpwd": true
        }
      ]
    }
  ]
}
```

### Indexing

{{% notice "note" %}}Filtering on a predicate by applying a [function]({{< relref "#functions" >}}) requires an index.{{% /notice %}}

When filtering by applying a function, Dgraph uses the index to make the search through a potentially large dataset efficient.

All scalar types can be indexed.  

Types `int`, `float`, `bool` and `geo` have only a default index each: with tokenizers named `int`, `float`, `bool` and `geo`.

Types `string` and `dateTime` have a number of indices.

#### String Indices
The indices available for strings are as follows.

| Index name / Tokenizer   | Purpose                                                             | Dgraph functions             |
| :----------- | :------------------------------------------------------------------ | :--------------------------- |
| `exact`      | matching of entire value                                            | `eq`, `le`, `ge`, `gt`, `lt` |
| `hash`       | matching of entire value, useful when the values are large in size  | `eq`                         |
| `term`       | matching of terms/words                                             | `eq`, `allofterms`, `anyofterms`   |
| `fulltext`   | matching with language specific stemming and stopwords              | `eq`, `alloftext`, `anyoftext`     |
| `trigram`    | regular expressions matching                                        | `regexp`                     |


#### Date Time Indices

**to be added after [issue #971](https://github.com/dgraph-io/dgraph/issues/971)**

#### Sortable Indices

Not all the indices establish a total order among the values that they index. Sortable indices allow inequality functions and sorting.

* Indexes `int` and `float` are sortable.  
* `string` index `exact` is sortable.
* All `dateTime` indices are sortable.

For example, given an edge `name` of `string` type, to sort by `name` or perform inequality filtering on names, the `exact` index must have been specified.  In which case a schema query would return at least the following tokenizers.

```
{
  "predicate": "name",
  "type": "string",
  "index": true,
  "tokenizer": [
    "exact"
  ]
}
```


### Adding or Modifying Schema

Schema mutations add or modify schema.

An index is specified with `@index`, with arguments to specify the tokenizer.  For example:

```
mutation {
  schema {
    name: string @index(exact, fulltext) .
    age: int @index .
    friend: uid .
    dob: dateTime .
    location: geo @index .
  }
}
```

If no data has been stored for the predicates, a schema mutation sets up an empty schema ready to receive triples.

If data is already stored before the mutation, existing values are not checked to conform to the new schema.  On query, Dgraph tries to convert existing values to the new schema types, ignoring any that fail conversion.

If data exists and new indices are specified in a schema mutation, any index not in the updated list is dropped and a new index is created for every new tokenizer specified.

Reverse edges are also computed if specified by a schema mutation.

### Reverse Edges

A graph edge is unidirectional. For node-node edges, sometimes modeling requires reverse edges.  If only some subject-predicate-object triples have a reverse, these must be manually added.  But if a predicate always has a reverse, Dgraph computes the reverse edges if `@reverse` is specified in the schema.

The reverse edge of `anEdge` is `~anEdge`.  

For existing data, Dgraph computes all reverse edges.  For data added after the schema mutation, Dgraph computes and stores the reverse edge for each added triple.

### Querying Schema

A schema query can query for the whole schema

```
schema { }
```

with particular schema fields

```
schema {
  type
  index
  reverse
  tokenizer
}
```

and for particular predicates

```
schema(pred: [name, friend]) {
  type
  index
  reverse
  tokenizer
}
```







## Mutations

























## OLD ---- still to be updated ----



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

If you want to delete all the objects/values of a `S P *` triple, you can do.
```
curl localhost:8080/query -XPOST -d $'
mutation {
  delete {
     <lewis-carrol> <died> * .
  }
}'
```
If you want to delete all the objects/values of all the predicates going out of S, you can do.
```
curl localhost:8080/query -XPOST -d $'
mutation {
  delete {
     <lewis-carrol> * * .
  }
}'
```
{{% notice "note" %}} On using `*`, all the derived edges (indexes, reverses) related to that edge would also be deleted.{{% /notice %}}










## Queries
For example, the query:

{{< runnable >}}
{
  # query block called df with root filter
  df(func: allofterms(name@en, "David Fincher")) {

    # literal about David Fincher
    name@en

    # block matching nodes found from current node
    # along director.film edge
    director.film  {  

      # literals for the film
      name@en
      initial_release_date

      # matching deeper
      genre {
        name@en
      }
    }
  }
}
{{< /runnable >}}

matches all nodes

Queries in GraphQL+- look very much like queries in GraphQL. You typically start with a node or a list of nodes, and expand edges from there.
Each `{}` block goes one layer deep.

{{< runnable >}}
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
{{< /runnable >}}

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




## Pagination
Often there is too much data and you only want a slice of the data.

### First

If you want just the first few results as you expand the predicate for an entity, you can use the `first` argument. The value of `first` can be positive for the first N results, and ''negative for the last N.''

{{% notice "note" %}}Without a sort order specified, the results are sorted by `_uid_`, which is assigned randomly. So the ordering while deterministic, might not be what you expected.{{% /notice  %}}

This query retrieves the first two films directed by Steven Spielberg and their last 3 genres.

{{< runnable >}}
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
{{< /runnable >}}

`first` can also be specified at root.

### Offset

If you want the '''next''' one result, you want to skip the first two results with `offset:2` and keep only one result  with `first:1`.

{{< runnable >}}
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
{{< /runnable >}}

Notice the `first` and `offset` arguments. `offset` can also be specified at root to skip over some results.

### After

Dgraph assigns `uint64`'s to all entities which are called UIDs (unique internal IDs). All results are sorted by UIDs by default. Therefore, another way to get the next one result after the first two results is to specify that the UIDs of all results are larger than the UID of the second result.

This helps in pagination where the first query would be of the form `<attribute>(after: 0x0, first: N)` and the subsequent ones will be of the form `<attribute>(after: <uid of last entity in last result>, first: N)`

In the above example, the first two results are the film entities of "Indiana Jones and the Temple of Doom" and "Jaws". You can obtain their UIDs by adding the predicate `_uid_` in the query.

{{< runnable >}}
{
  me(id: m.06pj8) {
    name@en
    director.film(first:2) {
      _uid_
      name@en
    }
  }
}
{{< /runnable >}}


Now we know the UID of the second result is `0xc6f4b3d7f8cbbad`. We can get the next one result by specifying the `after` argument.

{{< runnable >}}
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
{{< /runnable >}}

The response is the same as before when we use `offset:2` and `first:1`.



## Alias

Alias lets us provide alternate names to predicates in results for convenience.

For example, the following query replaces the predicate `name` with `full_name` and _uid_ with id in the JSON result.
{{< runnable >}}
{
  me(id: m.0bxtg) {
    id: _uid_
    full_name:name@en
  }
}
{{< /runnable >}}

## Count

`count` function lets us obtain the number of entities instead of retrieving the entire list. For example, the following query
retrieves the name and the number of films acted by an actor with `_xid_` m.0bxtg.

{{< runnable >}}
{
  me(id: m.0bxtg) {
    name@en
    count(actor.film)
  }
}
{{< /runnable >}}

It can also be used to get count at root. The following query would get the count of all directors.
{{< runnable >}}
{
  directors(func: gt(count(director.film), 0)) {
    count()
  }
}
{{< /runnable >}}




## Sorting

We can sort results by a predicate using the `orderasc` or `orderdesc` argument. The predicate has to be indexed and this has to be specified in the schema. As you may expect, `orderasc` sorts in ascending order while `orderdesc` sorts in descending order.

For example, we can sort the films of Steven Spielberg by their release date, in ascending order.
{{< runnable >}}
{
  me(id: m.06pj8) {
    name@en
    director.film(orderasc: initial_release_date) {
      name@en
      initial_release_date
    }
  }
}
{{< /runnable >}}

If you use `orderdesc` instead, the films will be listed in descending order.
{{< runnable >}}
{
  me(id: m.06pj8) {
    name@en
    director.film(orderdesc: initial_release_date, first: 2) {
      name@en
      initial_release_date
    }
  }
}
{{< /runnable >}}


To sort at root level, we can do as follows:
{{< runnable >}}
{
  me(func: allofterms(name, "ste"), orderasc: name) {
    name@en
  }
}
{{< /runnable >}}


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
`allofterms, ge, eq etc.` with facets.

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

### Assigning Facets to a variable

Facets that are a part of UID edges can be stored to a variable and used akin value variables.
```
curl localhost:8080/query -XPOST -d $'{
  var(id:<alice>) {
    friend @facets(a as close, b as relative)
  }

	friend(id: var(a)) {
		name
		var(a)
	}

	relative(id: var(b)) {
		name
		var(b)
	}
}' | python -m json.tool | less
```
Output:
```
{
  "friend": [
    {
      "name": "bob",
      "var(a)": true
    },
    {
      "name": "dave",
      "var(a)": true
    },
    {
      "name": "charlie",
      "var(a)": false
    }
  ],
  "relative": [
    {
      "name": "bob",
      "var(b)": false
    },
    {
      "name": "dave",
      "var(b)": true
    },
    {
      "name": "charlie",
      "var(b)": true
    }
  ]
}
```

## Aggregation
Aggregation functions that are supported are `min, max, sum, avg`. While min and max operate on all scalar-values, sum and avg can operate only on `int and float` values. These functions can only be applied on variables. Aggregation does not depend on the index.

{{% notice "note" %}}We support aggregation on scalar value variables only.{{% /notice %}}

### Min

{{< runnable >}}
{
  director(id: m.06pj8) {
    director.film {
    	x as initial_release_date
    }
    min(var(x))
  }
}
{{< /runnable >}}

### Max

{{< runnable >}}
{
  director(id: m.06pj8) {
    director.film {
    	x as initial_release_date
    }
    max(var(x))
  }
}
{{< /runnable >}}

### Sum, Avg
In this example we get the sum and the average of the count of genres for movies directed by Steven Spielberg.
{{< runnable >}}
{
  director(func: allofterms(name, "steven spielberg")) {
    name@en
    director.film {
      g as count(genre)
    }
    sum(var(g))
    avg(var(g))
  }
}
{{< /runnable >}}


## Multiple Query Blocks
Multiple blocks can be inside a single query and they would be returned in the result with the corresponding block names.

{{< runnable >}}
{
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
    director.film @filter(ge(initial_release_date, "2008"))  {
        Release_date: initial_release_date
        Name: name@en
    }
  }
}
{{< /runnable >}}

### Var Blocks
Var blocks are the blocks which start with the keyword `var` and these blocks are not returned in the query results.

{{< runnable >}}
{
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
}
{{< /runnable >}}

## Query Variables

Variables can be defined at different levels of the query using the keyword `AS` and would contain the result list at that level as its value. These variables can be used in other query blocks or the same block (as long as it is used in the child nodes).

{{< runnable >}}
{
 var(func:allofterms(name, "angelina jolie")) {
   actor.film {
    A AS performance.film {  # All the films done by Angelina Jolie
     B As genre  # Genres of all the films done by Angelina Jolie
    }
   }
  }

 var(func:allofterms(name, "brad pitt")) {
   actor.film {
    C AS performance.film {  # All the films done by Brad Pitt
     D as genre  # Genres of all the films done by Brad Pitt
    }
   }
  }

 films(id: var(D)) @filter(var(B)) {   # Genres done by both Angelina and Brad
  name@en
   ~genre @filter(var(A) OR var(C)) {  # Movies done by either.
     name@en
   }
 }
}
{{< /runnable >}}

## Value Variables

Value variables are those which store the scalar values (unlike the UID lists which we saw above). These are a map from the UID to the corresponding value. They can store scalar predicates, aggregate functions, can be used for sorting results and retrieving. For example:

{{< runnable >}}
{
  var(func:allofterms(name, "angelina jolie")) {
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
      n as name
      m as initial_release_date
    }
    min(var(n))
    max(var(n))
    min(var(m))
    max(var(m))
  }
}
{{< /runnable >}}

This query shows a mix of how things can be used.

Facets can also be stored in value variables, but exactly one facet has to be specified.

{{% notice "note" %}} Value variables can be used in place of UID variables, in which case the UIDs would be extracted from it.{{% /notice %}}


## Aggregating value variables

Value variables can be combined using complex mathematical functions to asscociate a score for the entities which could then be used to order the entites or perform other operations on them. This can be useful for building newsfeeds, simple recommendation systems and the likes.

All these statements must be enclosed within a `math( <exp> )` block.

The supported operators are as follows:

| Operators                       | Types accepted                                 | What it does                                                   |
| :------------:                  | :--------------:                               | :------------------------:                                     |
| `+` `-` `*` `/` `%`             | int, float                                     | performs the corresponding operation                           |
| `min` `max`                     | All types except geo, bool  (binary functions) | selects the min/max value among the two                        |
| `<` `>` `<=` `>=` `==` `!=`     | All types except geo, bool                     | Returns true or false based on the values                      |
| `floor` `ceil``ln` `exp` `sqrt` | int, float (unary function)                    | performs the corresponding operation                           |
| `since`                         | date, datetime                                 | Returns the number of seconds in float from the time specified |
| `pow(a, b)`                     | int, float                                     | Returns `a to the power b`                                     |
| `logbase(a,b)`                  | int, float                                     | Returns `log(a)` to the base `b`                               |
| `cond(a, b, c)`                 | first operand must be a boolean                | selects `b` if `a` is true else `c`                            |

A simple example is:

{{< runnable >}}
{
	var(func:allofterms(name, "steven spielberg")) {
		name@en
		films as director.film {
			p as count(starring)
			q as count(genre)
			r as count(country)
			score as math(p + q + r)
		}
	}

	TopMovies(id: var(films), orderdesc: var(score), first: 5){
		name@en
		var(score)
	}
}
{{< /runnable >}}

In the above query we retrieve the top movies (by sum of number of actors, genres, countries) of the entity named steven spielberg.


Value variables and aggregations of them can be used in filters.  For example, if we want to add a condition based on release date to penalize movies that are more than 10 years old and want all scores greater than some value, we could do:

{{< runnable >}}
{
  var(func:allofterms(name, "steven spielberg")) {
    name@en
    films as director.film {
      p as count(starring)
      q as count(genre)
      date as initial_release_date
      years as math(since(date)/(365*24*60*60))
      score as math(cond(years > 10, 0, ln(p)+q-ln(years)))
    }
  }

  TopMovies(id: var(films), orderdesc: var(score)) @filter(gt(var(score), 2)){
    name@en
    var(score)
    var(date)
  }
}
{{< /runnable >}}



To aggregate the values over level we can do something like
{{< runnable >}}
{
	steven as var(func:allofterms(name, "steven spielberg")) {
		name@en
		director.film {
			p as count(starring)
			q as count(genre)
			r as count(country)
			score as math(p + q + r)
		}
		directorScore as sum(var(score))
	}

	score(id: var(steven)){
		name@en
		var(directorScore)
	}
}
{{< /runnable >}}

Here `directorScore` would be sum of `score` of all the movies directed by a director.



## GroupBy

A `groupby` query aggregates query results given a set of properties on which to group elements.  For example, a query containing the block `friend @groupby(age) { count(_uid_) }`, finds all nodes reachable along the friend edge, partitions these into groups based on age, then counts how many nodes are in each group.  The returned result is the grouped edges and the aggregations.

Inside a `groupby` block, only aggregations are allowed and `count` may only be applied to `_uid_`.   

It is often necessary to use `groupby` in conjunction with variables to extract information other than the grouped or aggregated edges.

For example, the following counts the number of movies in each genre and for each of those genres returns the genre name and the count.  The name can't be extracted in the `groupby` because it is not an aggregate, but `var(a)` can be used in its function as a UID to value map to organize the `byGenre` query by genre UID.  

{{< runnable >}}
{
  var(func:allofterms(name, "steven spielberg")) {
    director.film @groupby(genre) {
      a as count(_uid_)
    }
  }

  byGenre(id: var(a), orderdesc: var(a)) {
    name
    total_movies : var(a)
  }
}
{{< /runnable >}}



## Expand Predicates

`_predicate_` can be used to retrieve all the predicates that go out of the nodes at that level. For example:

{{< runnable >}}
{
  director(func: allofterms(name, "steven spielberg")) {
    _predicate_
  }
}
{{< /runnable >}}

The number of predicates from a node can be counted and be aliased.

{{< runnable >}}
{
  director(func: allofterms(name, "steven spielberg")) {
    num_predicates: count(_predicate_)
    my_predicates: _predicate_
  }
}
{{< /runnable >}}

This can be stored in a variable and passed to `expand()` function to expand all the predicates in that list.

{{< runnable >}}
{
  var(func: allofterms(name@en, "steven spielberg")) {
    name
    pred as _predicate_
  }

  director(func: allofterms(name@en, "steven spielberg")) {
    expand(var(pred)) {
      expand(_all_) # Expand all the predicates at this level
    }
  }
}
{{< /runnable >}}

If `_all_` is passed as an argument to `expand()`, all the predicates at that level would be retrieved. More levels can be specfied in a nested fashion under `expand()`.

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
{{< runnable >}}
{
	recurse(func: gt(count(~genre), 30000), first: 1){
		name@en
		~genre (first:10) @filter(gt(count(starring), 2))
		starring (first: 2)
		performance.actor
	}
}
{{< /runnable >}}
Some points to keep in mind while using recurse queries are:

- Each edge would be traversed only once. Hence, cycles would be avoided.
- You can specify only one level of predicates after root. These would be traversed recursively. Both scalar and entity-nodes are treated similarly.
- Only one recurse block is advised per query.
- Be careful as the result size could explode quickly and an error would be returned if the result set gets too large. In such cases use more filter, limit resutls using pagination, or provide a depth parameter at root as follows:

{{< runnable >}}
{
	recurse(func: gt(count(~genre), 30000), depth: 2){
		name@en
		~genre (first:2) @filter(gt(count(starring), 2))
		starring (first: 2)
		performance.actor
	}
}
{{< /runnable >}}

## Cascade Directive

`@cascade` directive forces a removal of those entites that don't have all the fields specified in the query. This can be useful in cases where some filter was applied. For example, consider this query:

{{< runnable >}}
{
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
}
{{< /runnable >}}

Here we also remove all the nodes that don't have a corresponding valid sibling node for `Warwick Davis`.

## Normalize directive

Queries can have a `@normalize` directive, which if supplied at the root, the response would only contain the predicates which are asked with an alias in the query. The response is also flatter and avoids nesting, hence would be easier to parse in some cases.

{{< runnable >}}
{
  director(func:allofterms(name, "steven spielberg")) @normalize {
    d: name@en
    director.film {
      f: name@en
      release_date
      starring(first: 2) {
        performance.actor {
          pa: name@en
        }
        performance.character {
          pc: name@en
        }
      }
      country {
        c: name@en
      }
    }
  }
}
{{< /runnable >}}

From the results we can see that since we didn't ask for `release_date` with an alias we didn't get it back. We got back all other combinations of movies directed by Steven Spielberg with all unique combinations of performance.actor, performance.character and country.

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

{{< runnable >}}
{
 "query": "query test($a: int, $b: int, $id: string){  me(id: $id) {name@en, director.film (first: $a, offset: $b) {name @en, genre(first: $a) { name@en }}}}",
 "variables" : {
  "$a": "5",
  "$b": "10",
  "$id": "[m.06pj8, m.0bxtg]"
 }
}
{{< /runnable >}}

* Variables whose type is suffixed with a `!` can't have a default value but must
have a value as part of the variables map.
* The value of the variable must be parsable to the given type, if not, an error is thrown.
* Any variable that is being used must be declared in the named query clause in the beginning.
* We also support default values for the variables. In the example below, `$a` has a
default value of `2`.

{{< runnable >}}
{
 "query": "query test($a: int = 2, $b: int!){  me(id: m.06pj8) {director.film (first: $a, offset: $b) {genre(first: $a) { name@en }}}}",
 "variables" : {
   "$a": "5",
   "$b": "10"
 }
}
{{< /runnable >}}

* If the variable is initialized in the variable map, the default value will be
overridden (In the example, `$a` will have value 5 and `$b` will be 3).

* The variable types that are supported as of now are: `int`, `float`, `bool`, `string` and `uid`.

{{% notice "note" %}}In GraphiQL interface, the query and the variables have to be separately entered in their respective boxes.{{% /notice %}}
