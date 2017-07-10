+++
title = "Query Language"
+++

Dgraph's GraphQL+- is based on Facebook's [GraphQL](https://facebook.github.io/graphql/).  GraphQL wasn't developed for Graph databases, but it's graph-like query syntax, schema validation and subgraph shaped response make it a great language choice.  We've modified the language to better support graph operations, adding and removing features to get the best fit for graph databases.  We're calling this simplified, feature rich language, ''GraphQL+-''.

GraphQL+- is a work in progress. We're adding more features and we might further simplify existing ones.

## Take a Tour - https://tour.dgraph.io

This document is the Dgraph query reference material.  It is not a tutorial.  It's designed as a reference for users who already know how to write queries in GraphQL+- but need to check syntax, or indices, or functions, etc.

{{% notice "note" %}}If you are new to Dgraph and want to learn how to use Dgraph and GraphQL+-, take the tour - https://tour.dgraph.io{{% /notice %}}


### Running examples

The examples in this reference use a database of 21 million triples about movies and actors.  The example queries run and return results.  The queries are executed by an instance of Dgraph running at https://play.dgraph.io/.  To run the queries locally or experiment a bit more, see the [Getting Started]({{< relref "get-started/index.md" >}}) guide, which also shows how to load the datasets used in the examples here.

## GraphQL+- Fundamentals

A GraphQL+- query finds nodes based on search criteria, matches patterns in a graph and returns a graph as a result.

A query is composed of nested blocks, starting with a query root.  The root finds the initial set of nodes against which the following graph matching and filtering is applied.


### Returning Values

Each query has a name, specified at the query root, and the same name identifies the results.

If an edge is of a value type, the value can be returned by giving the edge name.

Query Example: In the example dataset, as well as edges that link movies to directors and actors, movies have a name, release date and identifiers for a number of well known movie databases.  This query, with name `bladerunner`, and root matching a movie name, returns those values for the early 80's sci-fi classic "Blade Runner".

{{< runnable >}}
{
  bladerunner(func: eq(name, "Blade Runner")) {
    _uid_
    name@en
    initial_release_date
    netflix_id
  }
}
{{< /runnable >}}

The query first searches the graph, using indexes to make the search efficient, for all nodes with a `name` edge equalling "Blade Runner".  For the found node the query then returns the listed outgoing edges.

Every node had a unique 64 bit identifier.  The `_uid_` edge in the query above returns that identifier.  If the required node is already known, then the function `uid` finds the node.  

Query Example: "Blade Runner" movie data found by UID.

{{< runnable >}}
{
  bladerunner(func: uid(0x3cf6ed367ae4fa80)) {
    _uid_
    name@en
    initial_release_date
    netflix_id
  }
}
{{< /runnable >}}

A query can match many nodes and return the values for each.

Query Example: All nodes that have either "Blade" or "Runner" in the name.

{{< runnable >}}
{
  bladerunner(func: anyofterms(name, "Blade Runner")) {
    _uid_
    name@en
    initial_release_date
    netflix_id
  }
}
{{< /runnable >}}

Multiple IDs can be specified in a list to the `uid` function.  

Query Example:
{{< runnable >}}
{
  movies(func: uid(0x3cf6ed367ae4fa80, 0x949c72529f812b1b)) {
    _uid_
    name@en
    initial_release_date
    netflix_id
  }
}
{{< /runnable >}}


### Expanding Graph Edges

A query expands edges from node to node by nesting query blocks with `{ }`.  

Query Example: The actors and characters played in "Blade Runner".  The query first finds the node with name "Blade Runner", then follows  outgoing `starring` edges to nodes representing an actor's performance as a character.  From there the `performance.actor` and `performance,character` edges are expanded to find the actor names and roles for every actor in the movie.
{{< runnable >}}
{
  brCharacters(func: eq(name, "Blade Runner")) {
    name@en
    initial_release_date
    starring {
      performance.actor {
        name@en  # actor name
      }
      performance.character {
        name@en  # character name
      }
    }
  }
}
{{< /runnable >}}


### Comments

Anything on a line following a `#` is a comment

### Applying Filters

The query root finds an initial set of nodes and the query proceeds by returning values and following edges to further nodes - any node reached in the query is found by traversal after the search at root.  The nodes found can be filtered by applying `@filter`, either after the root or at any edge.

Query Example: "Blade Runner" director Ridley Scott's movies released before the year 2000.
{{< runnable >}}
{
  scott(func: eq(name, "Ridley Scott")) {
    name@en
    initial_release_date
    director.film @filter(le(initial_release_date, "2000")) {
      name@en
      initial_release_date
    }
  }
}
{{< /runnable >}}

Query Example: Movies with either "Blade" or "Runner" in the title and released before the year 2000.

{{< runnable >}}
{
  bladerunner(func: anyofterms(name, "Blade Runner")) @filter(le(initial_release_date, "2000")) {
    _uid_
    name@en
    initial_release_date
    netflix_id
  }
}
{{< /runnable >}}

### Language Support

Dgraph supports UTF-8 strings.  

In a query, for a string valued edge `edge`, the syntax
```
edge@lang1:...:langN
```
specifies the preference order for returned languages, with the following rules.

* At most one result will be returned.
* The preference list is considered left to right: if a value in given language is not found, the next language from the list is considered.
* If there are no values in any of the specified languages, no value is returned.
* A final `.` means that the a value without a specified language is returned or if there is no value without language, a value in ''some'' language is returned.

For example:

- `name`   => Look for an untagged string; return nothing if no untagged value exits.
- `name@.` => Look for an untagged string, then any language.
- `name@en` => Look for `en` tagged string; return nothing if no `en` tagged string exists.
- `name@en:.` => Look for `en`, then untagged, then any language.
- `name@en:pl` => Look for `en`, then `pl`, otherwise nothing.
- `name@en:pl:.` => Look for `en`, then `pl`, then untagged, then any language.


{{% notice "note" %}}In functions, language lists and `.` notation are not allowed.  In functions, a string edge without a language tag means apply function to all languages, while a single language tag means apply to only the given language.{{% /notice %}}


Query Example: Some of Bollywood director and actor Farhan Akhtar's movies have a name stored in Russian as well as Hindi and English, others do not.

{{< runnable >}}
{
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
}
{{< /runnable >}}




## Functions

{{% notice "note" %}}Functions can only be applied to [indexed]({{< relref "#indexing">}}) predicates.{{% /notice %}}

Functions allow filtering based on properties of nodes or variables.  Functions can be applied in the query root or in filters.

For functions on string valued predicates, if no language preference is given, the function is applied to all languages and strings without a language tag; if a language preference is given, the function is applied only to strings of the given language.


### Term matching


#### AllOfTerms

Syntax Example: `allofterms(predicate, "space-separated term list")`

Schema Types: `string`

Index Required: `term`


Matches strings that have all specified terms in any order; case insensitive.

##### Usage at root

Query Example: All nodes that have `name` containing terms `indiana` and `jones`, returning the english name and genre in english.

{{< runnable >}}
{
  me(func: allofterms(name@en, "jones indiana")) {
    name@en
    genre {
      name@en
    }
  }
}
{{< /runnable >}}

##### Usage as Filter

Query Example: Steven Spielberg is UID `0x3b0de646eaf32b75`.  All his films that contain the words `indiana` and `jones`.

{{< runnable >}}
{
  me(func: uid(0x3b0de646eaf32b75)) {
    name@en
    director.film @filter(allofterms(name@en, "jones indiana"))  {
      name@en
    }
  }
}
{{< /runnable >}}


#### AnyOfTerms


Syntax Example: `anyofterms(predicate, "space-separated term list")`

Schema Types: `string`

Index Required: `term`


Matches strings that have any of the specified terms in any order; case insensitive.

##### Usage at root

Query Example: All nodes that have a `name` containing either `poison` or `peacock`.  Many of the returned nodes are movies, but people like Joan Peacock also meet the search terms because without a [cascade directive]({{< relref "#cascade-directive">}}) the query doesn't require a genre.

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

Query Example: Steven Spielberg is UID `0x3b0de646eaf32b75`.  All his movies that contain `war` or `spies`

{{< runnable >}}
{
  me(func: uid(0x3b0de646eaf32b75)) {
    name@en
    director.film @filter(anyofterms(name, "war spies"))  {
      name@en
    }
  }
}
{{< /runnable >}}


### Regular Expressions


Syntax Examples: `regexp(predicate, /regular-expression/)` or case insensitive `regexp(predicate, /regular-expression/i)`

Schema Types: `string`

Index Required: `trigram`


Matches strings by regular expression.  The regular expression language is that of [go regular expressions](https://golang.org/pkg/regexp/syntax/).

Query Example: At root, match nodes with `Steven Sp` at the start of `name`, followed by any characters.  For each such matched uid, match the films containing `ryan`.  Note the difference with `allofterms`, which would match only `ryan` but regular expression search will also match within terms, such as `bryan`.

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

Syntax Examples: `alloftext(predicate, "space-separated text")` and `anyoftext(predicate, "space-separated text")`

Schema Types: `string`

Index Required: `fulltext`


Apply full text search with stemming and stop words to find strings matching all or any of the given text.  

The following steps are applied during index generation and to process full text search arguments:

1. Tokenization (according to Unicode word boundaries).
1. Conversion to lowercase.
1. Unicode-normalization (to [Normalization Form KC](http://unicode.org/reports/tr15/#Norm_Forms)).
1. Stemming using language-specific stemmer.
1. Stop words removal

Dgraph uses [bleve](https://github.com/blevesearch/bleve) for its full text search indexing.  See also the bleve language specific [stop word lists](https://github.com/blevesearch/bleve/tree/master/analysis/lang).

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


Query Example: All names that have `run`, `running`, etc and `man`.  Stop word removal eliminates `the` and `maybe`

{{< runnable >}}
{
  movie(func:alloftext(name@en, "the man maybe runs")) {
	 name@en
  }
}
{{< /runnable >}}


### Inequality

#### equal to

Syntax Examples:

* `eq(predicate, value)`
* `eq(var(varName), value)`
* `eq(count(predicate), value)`
* `eq(predicate, [val1, val2, ..., valN])`

Schema Types: `int`, `float`, `bool`, `string`, `dateTime`

Index Required: An index is required for the `eq(predicate, ...)` forms (see table below).  For `count(predicate)` at the query root, the `@count` index is required. For variables the values have been calculated as part of the query, so no index is required.  

| Type       | Index Options |
|:-----------|:--------------|
| `int`      | `int`         |
| `float`    | `float`       |
| `bool`     | `bool`        |
| `string`   | `exact`, `hash`, `term`, `fulltext` |
| `dateTime` | `dateTime`    |

Test for equality of a predicate or variable to a value or find in a list of values.

The boolean constants are `true` and `false`, so with `eq` this becomes, for example, `eq(boolPred, true)`.

Query Example: Movies with exactly thirteen genres.

{{< runnable >}}
{
  me(func: eq(count(genre), 13)) {
    name@en
    genre {
    	name@en
    }
  }
}
{{< /runnable >}}


Query Example: Directors called Steven who have directed 1,2 or 3 movies.

{{< runnable >}}
{
  steve as var(func: allofterms(name@en, "Steven")) {
    films as count(director.film)
  }

  stevens(id: var(steve)) @filter(eq(var(films), [1,2,3])) {
    name@en
    numFilms : var(films)
  }
}
{{< /runnable >}}


#### Less than, less than or equal to, greater than and greater than or equal to

Syntax Examples: for inequality `IE`

* `IE(predicate, value)`
* `IE(var(varName), value)`
* `IE(count(predicate), value)`

With `IE` replaced by

* `le` less than or equal to
* `lt` less than
* `ge` greater than or equal to
* `gt` greather than

Schema Types: `int`, `float`, `string`, `dateTime`

Index required: An index is required for the `IE(predicate, ...)` forms (see table below).  For `count(predicate)` at the query root, the `@count` index is required. For variables the values have been calculated as part of the query, so no index is required.

| Type       | Index Options |
|:-----------|:--------------|
| `int`      | `int`         |
| `float`    | `float`       |
| `string`   | `exact`       |
| `dateTime` | `dateTime`    |


Query Example: Steven Spielberg is UID `0x3b0de646eaf32b75`.  All his movies released before 1970.

{{< runnable >}}
{
  me(func: uid(0x3b0de646eaf32b75)) {
    name@en
    director.film @filter(lt(initial_release_date, "1970-01-01"))  {
      initial_release_date
      name@en
    }
  }
}
{{< /runnable >}}


Query Example: Movies with directors with `Steven` in `name` and have directed more than `100` actors.

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



Query Example: A movie in each genre that has over 30000 movies.  Because there is no order specified on genres, the order will be by UID.  The [count index]({{< relref "#count-index">}}) records the number of edges out of nodes and makes such queries more .

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


### Has

Syntax Examples: `has(predicate)`

Schema Types: all

Index Required: `count` (when used at query root)

Determines if a node has a particular predicate.



Query Example: First five directors and all their movies that have a release date recorded.  Directors have directed at least one film --- equivalent semantics to `gt(count(director.film), 0)`.
{{< runnable >}}
{
  me(func: has(director.film), first: 5) {
    name@en
    director.film @filter(has(initial_release_date))  {
      initial_release_date
      name@en
    }
  }
}
{{< /runnable >}}

### Geolocation

Note that for geo queries, any polygon with holes is replace with the outer loop, ignoring holes.  Also, as for version 0.7.7 polygon containment checks are approximate.

#### Near

Syntax Example: `near(predicate, [long, lat], distance)`

Schema Types: `geo`

Index Required: `geo`

Matches all entities where the location given by `predicate` is within `distance` metres of geojson coordinate `[long, lat]`.

Query Example: Tourist destinations within 1 kilometer of a point in Golden Gate Park, San Fransico.

{{< runnable >}}
{
  tourist(func: near(loc, [-122.469829, 37.771935], 1000) ) {
    name
  }
}
{{< /runnable >}}


#### Within

Syntax Example: `within(predicate, [[long1, lat1], ..., [longN, latN]])`

Schema Types: `geo`

Index Required: `geo`

Matches all entities where the location given by `predicate` lies within the polygon specified by the geojson coordinate array.

Query Example: Tourist destinations within the specified area of Golden Gate Park, San Fransico.  

{{< runnable >}}
{
  tourist(func: within(loc, [[-122.47266769409178, 37.769018558337926 ], [ -122.47266769409178, 37.773699921075135 ], [ -122.4651575088501, 37.773699921075135 ], [ -122.4651575088501, 37.769018558337926 ], [ -122.47266769409178, 37.769018558337926]] )) {
    name
  }
}
{{< /runnable >}}


#### Contains

Syntax Examples: `contains(predicate, [long, lat])` or `contains(predicate, [[long1, lat1], ..., [longN, latN]])`

Schema Types: `geo`

Index Required: `geo`

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

Syntax Example: `intersects(predicate, [[long1, lat1], ..., [longN, latN]])`

Schema Types: `geo`

Index Required: `geo`

Matches all entities where the polygon describing the location given by `predicate` intersects the given geojson polygon.


{{< runnable >}}
{
  tourist(func: intersects(loc, [[-122.503325343132, 37.73345766902749 ], [ -122.503325343132, 37.733903134117966 ], [ -122.50271648168564, 37.733903134117966 ], [ -122.50271648168564, 37.73345766902749 ], [ -122.503325343132, 37.73345766902749]] )) {
    name
  }
}
{{< /runnable >}}



## Connecting Filters

Within `@filter` multiple functions can be used with boolean connectives.

### AND, OR and NOT

Connectives `AND`, `OR` and `NOT` join filters and can be built into arbitrarily complex filters, such as `(NOT A OR B) AND (C AND NOT (D OR E))`.  Note that, `NOT` binds more tightly than `AND` which binds more tightly than `OR`.

Query Example : Steven Spielberg is UID `0x3b0de646eaf32b75`.  All his movies that contain either both "indiana" and "jones" OR both "jurassic" and "park".

{{< runnable >}}
{
  me(func: uid(0x3b0de646eaf32b75)) {
    name@en
    director.film @filter(allofterms(name, "jones indiana") OR allofterms(name, "jurassic park"))  {
      _uid_
      name@en
    }
  }
}
{{< /runnable >}}


## Alias

Syntax Examples:

* `aliasName : predicate`
* `aliasName : predicate { ... }`
* `aliasName : varName as ...`
* `aliasName : count(predicate)`
* `aliasName : max(var(varName))`

An alias provides an alternate name in results.  Predicates, variables and aggregates can be aliased by prefixing with the alias name and `:`.  Aliases do not have to be different to the original predicate name, but, within a block, an alias must be distinct from predicate names and other aliases returned in the same block.  Aliases can be used to return the same predicate multiple times within a block.  



Query Example: Directors with `name` matching term `Steven`, their UID, english name, average number of actors per movie, total number of films and the name of each film in english and french.  
{{< runnable >}}
{
  ID as var(func: allofterms(name@en, "Steven")) @filter(has(director.film)) {
    director.film {
      num_actors as count(starring)
    }
    average as avg(var(num_actors))
  }

  films(id: var(ID)) {
    director_id : _uid_
    english_name : name@en
    average_actors : var(average)
    num_films : count(director.film)

    films : director.film {
      name : name@en
      english_name : name@en
      french_name : name@fr
    }
  }
}
{{< /runnable >}}


## Pagination

Pagination allows returning only a portion, rather than the whole, result set.  This can be useful for top-k style queries as well as to reduce the size of the result set for client side processing or to allow paged access to results.

Pagination is often used with [sorting]({{< relref "#sorting">}}).  

{{% notice "note" %}}Without a sort order specified, the results are sorted by `_uid_`, which is assigned randomly. So the ordering, while deterministic, might not be what you expected.{{% /notice  %}}

### First

Syntax Examples:

* `q(func: ..., first: N)`
* `predicate (first: N) { ... }`
* `predicate @filter(...) (first: N) { ... }`

For positive `N`, `first: N` retrieves the first `N` results, by sorted or UID order.  

For negative `N`, `first: N` retrieves the last `N` results, by sorted or UID order.  Currently, negative is only supported when no order is applied.  To achieve the effect of a negative with a sort, reverse the order of the sort and use a positive `N`.


Query Example: Last two films, by UID order, directed by Steven Spielberg and the first 3 genres, sorted alphabetically by English name, of those movies.

{{< runnable >}}
{
  me(func: allofterms(name@en, "Steven Spielberg")) {
    director.film (first: -2) {
      name@en
      initial_release_date
      genre (orderasc: name@en) (first: 3) {
          name@en
      }
    }
  }
}
{{< /runnable >}}



Query Example: The three directors with name Steven who have directed the least actors of all directors named Steven.

{{< runnable >}}
{
  ID as var(func: allofterms(name@en, "Steven")) {
    director.film {
      stars as count(starring)
    }
    totalActors as sum(var(stars))
  }

  leastStars(id: var(ID), orderasc: var(totalActors), first: 3) {
    name@en
    stars : var(totalActors)

    director.film {
      name@en
    }
  }
}
{{< /runnable >}}

### Offset

Syntax Examples:

* `q(func: ..., offset: N)`
* `predicate (offset: N) { ... }`
* `predicate (first: M, offset: N) { ... }`
* `predicate @filter(...) (offset: N) { ... }`

With `offset: N` the first `N` results are not returned.  Used in combination with first, `first: M, offset: N` skips over `N` results and returns the following `M`.

Query Example: Order Hark Tsui's films by English title, skip over the first 4 and return the following 6.

{{< runnable >}}
{
  me(func: allofterms(name@en, "Hark Tsui")) {
    name@zh
    name@en
    director.film (orderasc: name@en) (first:6, offset:4)  {
      genre {
        name@en
      }
      name@zh
      name@en
      initial_release_date
    }
  }
}
{{< /runnable >}}

### After

Syntax Examples:

* `q(func: ..., after: UID)`
* `predicate (first: N, after: UID) { ... }`
* `predicate @filter(...) (first: N, after: UID) { ... }`

Another way to get results after skipping over some results is to use the default UID ordering and skip directly past a node specified by UID.  For example, a first query could be of the form `predicate (after: 0x0, first: N)`, or just `predicate (first: N)`, with subsequent queries of the form `predicate(after: <uid of last entity in last result>, first: N)`.


Query Example: The first five of Baz Luhrmann's films, sorted by UID order.

{{< runnable >}}
{
  me(func: allofterms(name@en, "Baz Luhrmann")) {
    name@en
    director.film (first:5) {
      _uid_
      name@en
    }
  }
}
{{< /runnable >}}

The fifth movie is the Australian movie classic Strictly Ballroom.  It has UID `0xeda1f2fe766ed92d`.  The results after Strictly Ballroom can now be obtained with `after`.

{{< runnable >}}
{
  me(func: allofterms(name@en, "Baz Luhrmann")) {
    name@en
    director.film (first:5, after: 0xeda1f2fe766ed92d) {
      _uid_
      name@en
    }
  }
}
{{< /runnable >}}


## Count

Syntax Examples:

* `count(predicate)`
* `count()`

The form `count(predicate)` counts how many `predicate` edges lead out of a node.

The form `count()` counts the number of UIDs matched in the enclosing block.

Query Example: The number of films acted in by each actor with `Orlando` in their name.

{{< runnable >}}
{
  me(func: allofterms(name@en, "Orlando")) @filter(has(actor.film)) {
    name@en
    count(actor.film)
  }
}
{{< /runnable >}}

Count can be used at root and [aliased]({{< relref "#alias">}}).

Query Example: Count of directors who have directed more than five films.  When used at the query root, the [count index]({{< relref "#count-index">}}) is required.

{{< runnable >}}
{
  directors(func: gt(count(director.film), 5)) {
    totalDirectors : count()
  }
}
{{< /runnable >}}


Count can be assigned to a [value variable]({{< relref "#value-variables">}}).

Query Example: The actors of Ang Lee's "Eat Drink Man Woman" ordered by the number of movies acted in.

{{< runnable >}}
{
	var(func: allofterms(name@en, "eat drink man woman")) {
    starring {
      actors as performance.actor {
        totalRoles as count(actor.film)
      }
    }
  }

  edmw(id: var(actors), orderdesc: var(totalRoles)) {
    name@en
    name@zh
    totalRoles : var(totalRoles)
  }
}
{{< /runnable >}}


## Sorting

Syntax Examples:

* `q(func: ..., orderasc: predicate)`
* `q(func: ..., orderdesc: var(varName))`
* `predicate (orderdesc: predicate) { ... }`
* `predicate @filter(...) (orderasc: N) { ... }`

Sortable Types: `int`, `float`, `String`, `dateTime`, `id`, `default`

Results can be sorted in ascending, `orderasc` or decending `orderdesc` order by a predicate or variable.

For sorting on predicates with [sortable indices]({{< relref "#sortable-indices">}}), Dgraph sorts on the values and with the index in parallel and returns whichever result is computed first.


Query Example: French director Jean-Pierre Jeunet's movies sorted by release date.

{{< runnable >}}
{
  me(func: allofterms(name, "Jean-Pierre Jeunet")) {
    name@fr
    director.film(orderasc: initial_release_date) {
      name@fr
      name@en
      initial_release_date
    }
  }
}
{{< /runnable >}}

Sorting can be performed at root and on value variables.

Query Example: All genres sorted alphabetically and the five movies in each genre with the most genres.

{{< runnable >}}
{
  genres as var(func: has(~genre)) {
    ~genre {
      numGenres as count(genre)
    }
  }

  blaa(id: var(genres), orderasc: name@en) {
    name@en
    ~genre (orderdesc: var(numGenres), first: 5) {
      name@en
    	genres : var(numGenres)
    }
  }
}
{{< /runnable >}}



## Multiple Query Blocks

Inside a single query, multiple query blocks are allowed.  The result is all blocks with corresponding block names.

Multiple query blocks are executed in parallel.

The blocks need not be related in any way.

Query Example: All of Angelina Jolie's films, with genres, and Steven Spielberg's (UID `0x3b0de646eaf32b75`) films since 2008.

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

 DirectorInfo(func: uid(0x3b0de646eaf32b75)) {
    name@en
    director.film @filter(ge(initial_release_date, "2008"))  {
        Release_date: initial_release_date
        Name: name@en
    }
  }
}
{{< /runnable >}}


If queries contain some overlap in answers, the result sets are still independent

Query Example: The movies Mackenzie Crook has acted in and the movies Jack Davenport has acted in.  The results sets overlap because both have acted in the Pirates of the Caribbean movies, but the results are independent and both contain the full answers sets.  

{{< runnable >}}
{
  Mackenzie(func:allofterms(name, "Mackenzie Crook")) {
    name@en
    actor.film {
      performance.film {
        _uid_
        name@en
      }
      performance.character {
        name@en
      }
    }
  }

  Jack(func:allofterms(name, "Jack Davenport")) {
    name@en
    actor.film {
      performance.film {
        _uid_
        name@en
      }
      performance.character {
        name@en
      }
    }
  }
}
{{< /runnable >}}


### Var Blocks

Var blocks start with the keyword `var` and are not returned in the query results.

Query Example: Angelina Jolie's movies ordered by genre.

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

  films(id: var(B), orderasc: name@en) {
    name@en
    ~genre @filter(var(A)) {
      name@en
    }
  }
}
{{< /runnable >}}


## Query Variables

Syntax Examples:

* `varName as q(func: ...) { ... }`
* `varName as var(func: ...) { ... }`
* `varName as predicate { ... }`
* `varName as predicate @filter(...) { ... }`

Types : `uid`

Nodes (UID's) matched at one place in a query can be stored in a variable and used elsewhere.  Query variables can be used in other query blocks or in a child node of the defining block.

Query variables do not affect the semantics of the query at the point of definition.  Query variables are evaluated to all nodes matched by the defining block.

In general, query blocks are executed in parallel, but variables impose an evaluation order on some blocks.  Cycles induced by variable dependence are not permitted.

If a variable is defined, it must be used elsewhere in the query.

The syntax `id: var(A,B)` or `@filter(var(A,B))` means the union of UIDs for variables `A` and `B`.

Query Example: The movies of Angelia Jolie and Brad Pitt where both have acted on movies in the same genre.  Note that `B` and `D` match all genres for all movies, not genres per movie.
{{< runnable >}}
{
 var(func:allofterms(name, "angelina jolie")) {
   actor.film {
    A AS performance.film {  # All films acted in by Angelina Jolie
     B As genre  # Genres of all the films acted in by Angelina Jolie
    }
   }
  }

 var(func:allofterms(name, "brad pitt")) {
   actor.film {
    C AS performance.film {  # All films acted in by Brad Pitt
     D as genre  # Genres of all the films acted in by Brad Pitt
    }
   }
  }

 films(id: var(D)) @filter(var(B)) {   # Genres from both Angelina and Brad
  name@en
   ~genre @filter(var(A, C)) {  # Movies in either A or C.
     name@en
   }
 }
}
{{< /runnable >}}


## Value Variables

Syntax Examples:

* `varName as scalarPredicate`
* `varName as count(predicate)`
* `varName as avg(...)`
* `varName as math(...)`

Types : `int`, `float`, `String`, `dateTime`, `id`, `default`, `geo`, `bool`

Value variables store scalar values.  Value variables are a map from the UIDs of the enclosing block to the corresponding values.

It therefor only makes sense to use a value variable in a context that matches the same UIDs - if used in a block matching different UIDs the value variable is undefined.

It is an error to define a value variable but not use it elsewhere in the query.

[Facet]({{< relref "#facets-edge-attributes">}}) values can be stored in value variables.

Query Example: The number of movie roles played by the actors of the 80's classic "The Princess Bride".  Query variable `pbActors` matches the UIDs of all actors from the movie.  Value variable `roles` is thus a map from actor UID to number of roles.  Value variable `roles` can be used in the the `totalRoles` query block because that query block also matches the `pbActors` UIDs, so the actor to number of roles map is available.

{{< runnable >}}
{
  var(func:allofterms(name, "The Princess Bride")) {
    starring {
      pbActors as performance.actor {
        roles as count(actor.film)
      }
    }
  }
  totalRoles(id: var(pbActors), orderasc: var(roles)) {
    name@en
    numRoles : var(roles)
  }
}
{{< /runnable >}}


Value variables can be used in place of UID variables, in which case they are treated as the UID list from the map.

Query Example: The same query as the previous example, but using value variable `roles` for matching UIDs in the `totalRoles` query block.

{{< runnable >}}
{
  var(func:allofterms(name, "The Princess Bride")) {
    starring {
      performance.actor {
        roles as count(actor.film)
      }
    }
  }
  totalRoles(id: var(roles), orderasc: var(roles)) {
    name@en
    numRoles : var(roles)
  }
}
{{< /runnable >}}


### Variable Propagation

Like query variables, value variables can be used in other query blocks and in blocks nested within the defining block.  When used in a block nested within the block that defines the variable, the value is computed as a sum of the variable for parent nodes along all paths to the point of use.  This is called variable propagation.

For example:
```
{
  q(id: 0x01) {
    myscore as math(1)          # A
    friends {                   # B
      friends {                 # C
        ...myscore...
      }
    }
  }
}
```
At line A, a value variable `myscore` is defined as mapping node with UID `0x01` to value 1.  At B, the value for each friend is still 1: there is only one path to each friend.  Traversing the friend edge twice reaches the friends of friends. The variable `myscore` gets propagated such that each friend of friend will receive the sum of its parents values:  if a friend of a friend is reachable from only one friend, the value is still 1, if they are reachable from two friends, the value is two and so on.  That is, the value of `myscore` for each friend of friends inside the block marked C will be the number of paths to them.  

**The value that a node receives for a propagated variable is the sum of the values of all its parent nodes.**

This propagation is useful, for example, in normalizing a sum across users, finding the number of paths between nodes and accumulating a sum through a graph.



Query Example: For each Harry Potter movie, the number of roles played by actor Warwick Davis.
{{< runnable >}}
{
	num_roles(func:eq(name, "Warwick Davis")) @cascade @normalize {

    paths as math(1)  # records number of paths to each character

    actor: name@en

    actor.film {
      performance.film @filter(allofterms(name, "Harry Potter")) {
        film_name : name@en
        characters : var(paths)  # how many paths (i.e. characters) reach this film
      }
    }
  }
}
{{< /runnable >}}


Query Example: Each actor who has been in a Peter Jackson movie and the fraction of Peter Jackson movies they have appeared in.  
{{< runnable >}}
{
	movie_fraction(func:eq(name, "Peter Jackson")) @normalize {

    paths as math(1)
    total_films : num_films as count(director.film)
    director : name@en

    director.film {
      starring {
        performance.actor {
          fraction : math(paths / (num_films/paths))
          actor : name@en
        }
      }
    }
  }
}
{{< /runnable >}}

More examples can be found in two Dgraph blog posts about using variable propagation for recommendation engines ([post 1](https://open.dgraph.io/post/recommendation/), [post 2](https://open.dgraph.io/post/recommendation2/)).

## Aggregation

Syntax Example: `AG(var(varName))`

For `AG` replaced with

* `min` : select the minimum value in the value variable `varName`
* `max` : select the maximum value
* `sum` : sum all values in value variable `varName`
* `avg` : calculate the average of values in `varName`

Schema Types:

| Aggregation       | Schema Types |
|:-----------|:--------------|
| `min` / `max`     | `int`, `float`, `string`, `dateTime`, `default`         |
| `sum` / `avg`    | `int`, `float`       |

Aggregation can only be applied to [value variables]({{< relref "#value-variables">}}).  An index is not required (the values have already been found and stored in the value variable mapping).

An aggregation is applied at the query block enclosing the variable definition.  As opposed to query variables and value variables, which are global, aggregation is computed locally.  For example:
```
A as predicateA {
  ...
  B as predicateB {
    x as ...some value...
  }
  min(var(x))
}
```
Here, `A` and `B` are the lists of all UIDs that match these blocks.  Value variable `x` is a mapping from UIDs in `B` to values.  The aggregation `min(var(x))`, however, is computed for each UID in `A`.  That is, it has a semantics of: for each UID in `A`, take the slice of `x` that corresponds to `A`'s outgoing `predicateB` edges and compute the aggregation for those values.

Aggregations can themselves be assigned to value variables, making a UID to aggregation map.


### Min

Query Example:  Directors called Steven and the date of release of their first movie, in ascending order of first movie.

{{< runnable >}}
{
  stevens as var(func: allofterms(name@en, "steven")) {
    director.film {
      ird as initial_release_date  
      # ird is a value variable mapping a film UID to its release date
    }
    minIRD as min(var(ird))
    # minIRD is a value variable mapping a director UID to their first release date
  }

  byIRD(id: var(stevens), orderasc: var(minIRD)) {
    name@en
    firstRelease: var(minIRD)
  }
}
{{< /runnable >}}

### Max

Query Example: Quentin Tarantino's movies and date of release of the most recent movie.

{{< runnable >}}
{
  director(func: allofterms(name@en, "Quentin Tarantino")) {
    director.film {
      name@en
      x as initial_release_date
    }
    max(var(x))
  }
}
{{< /runnable >}}

### Sum and Avg

Query Example: Steven Spielberg's movies, with the number of recorded genres per movie, and the total number of genres and average genres per movie.

{{< runnable >}}
{
  director(func: eq(name@en, "Steven Spielberg")) {
    name@en
    director.film {
      name@en
      numGenres : g as count(genre)
    }
    totalGenres : sum(var(g))
    genresPerMovie : avg(var(g))
  }
}
{{< /runnable >}}


### Aggregating Aggregates

Aggregations can be assigned to value variables, and so these variables can in turn be aggregated.

Query Example: For each actor in a Peter Jackson film, find the number of roles played in any movie.  Sum these to find the total number of roles ever played by all actors in the movie.  Then sum the lot to find the total number of roles ever played by actors who have appeared in Peter Jackson movies.  Note that this demonstrates how to aggregate aggregates; the answer in this case isn't quite precise though, because actors that have appeared in multiple Peter Jackson movies are counted more than once.

{{< runnable >}}
{
  PJ as var(func:allofterms(name, "Peter Jackson")) {
    director.film {
      starring {  # starring an actor
        performance.actor {
          movies as count(actor.film)  
          # number of roles for this actor
        }
        perf_total as sum(var(movies))       
      }
      movie_total as sum(var(perf_total))
      # total roles for all actors in this movie
    }
    gt as sum(var(movie_total))
  }

  PJmovies(id: var(PJ)) {
    name@en
  	director.film (orderdesc: var(movie_total), first: 5) {
    	name@en
    	totalRoles : var(movie_total)
  	}
    grandTotal : var(gt)
  }
}
{{< /runnable >}}


## Math on value variables

Value variables can be combined using mathematical functions.  For example, this could be used to associate a score which is then be used to order or perform other operations, such as might be used in building newsfeeds, simple recommendation systems and the likes.

Math statements must be enclosed within `math( <exp> )` and must be stored to a value variable.

The supported operators are as follows:

| Operators                       | Types accepted                                 | What it does                                                   |
| :------------:                  | :--------------:                               | :------------------------:                                     |
| `+` `-` `*` `/` `%`             | `int`, `float`                                     | performs the corresponding operation                           |
| `min` `max`                     | All types except `geo`, `bool`  (binary functions) | selects the min/max value among the two                        |
| `<` `>` `<=` `>=` `==` `!=`     | All types except `geo`, `bool`                     | Returns true or false based on the values                      |
| `floor` `ceil` `ln` `exp` `sqrt` | `int`, `float` (unary function)                    | performs the corresponding operation                           |
| `since`                         | `dateTime`                                 | Returns the number of seconds in float from the time specified |
| `pow(a, b)`                     | `int`, `float`                                     | Returns `a to the power b`                                     |
| `logbase(a,b)`                  | `int`, `float`                                     | Returns `log(a)` to the base `b`                               |
| `cond(a, b, c)`                 | first operand must be a boolean                | selects `b` if `a` is true else `c`                            |


Query Example:  Form a score for each of Steven Spielberg's movies as the sum of number of actors, number of genres and number of countries.  List the top five such movies in order of decreasing score.

{{< runnable >}}
{
	var(func:allofterms(name, "steven spielberg")) {
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

Value variables and aggregations of them can be used in filters.

Query Example: Calculate a score for each Steven Spielberg movie with a condition on release date to penalize movies that are more than 10 years old, filtering on the resulting score.

{{< runnable >}}
{
  var(func:allofterms(name, "steven spielberg")) {
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


Values calculated with math operations are stored to value variables and so can be aggreated.

Query Example: Compute a score for each Steven Spielberg movie and then aggregate the score.

{{< runnable >}}
{
	steven as var(func:eq(name, "Steven Spielberg")) @filter(has(director.film)) {
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


## GroupBy

Syntax Examples:

* `q(func: ...) @groupby(predicate) { min(...) }`
* `predicate @groupby(pred) { count(_uid_) }``


A `groupby` query aggregates query results given a set of properties on which to group elements.  For example, a query containing the block `friend @groupby(age) { count(_uid_) }`, finds all nodes reachable along the friend edge, partitions these into groups based on age, then counts how many nodes are in each group.  The returned result is the grouped edges and the aggregations.

Inside a `groupby` block, only aggregations are allowed and `count` may only be applied to `_uid_`.   

If the `groupby` is applied to a `uid` predicate, the resulting aggregations can be saved in a variable (mapping the grouped UIDs to aggregate values) and used elsewhere in the query to extract information other than the grouped or aggregated edges.

Query Example: For Steven Spielberg movies, count the number of movies in each genre and for each of those genres return the genre name and the count.  The name can't be extracted in the `groupby` because it is not an aggregate, but `var(a)` can be used in its function as a UID to value map to organize the `byGenre` query by genre UID.  


{{< runnable >}}
{
  var(func:allofterms(name, "steven spielberg")) {
    director.film @groupby(genre) {
      a as count(_uid_)
      # a is a genre UID to count value variable
    }
  }

  byGenre(id: var(a), orderdesc: var(a)) {
    name@en
    total_movies : var(a)
  }
}
{{< /runnable >}}

Query Example: Actors from Tim Burton movies and how many roles they have played in Tim Burton movies.
{{< runnable >}}
{
  var(func:allofterms(name, "Tim Burton")) {
    director.film {
      starring @groupby(performance.actor) {
        a as count(_uid_)
        # a is an actor UID to count value variable
      }
    }
  }

  byActor(id: var(a), orderdesc: var(a)) {
    name@en
    var(a)
  }
}
{{< /runnable >}}



## Expand Predicates

Keyword `_predicate_` retrieves all predicates out of nodes at the level used.

Query Example: All predicates from actor Geoffrey Rush.
{{< runnable >}}
{
  director(func: eq(name, "Geoffrey Rush")) {
    _predicate_
  }
}
{{< /runnable >}}

The number of predicates from a node can be counted and be aliased.

Query Example: All predicates from actor Geoffrey Rush and the count of such predicates.
{{< runnable >}}
{
  director(func: eq(name, "Geoffrey Rush")) {
    num_predicates: count(_predicate_)
    my_predicates: _predicate_
  }
}
{{< /runnable >}}

Predicates can be stored in a variable and passed to `expand()` to expand all the predicates in the variable.

If `_all_` is passed as an argument to `expand()`, all the predicates at that level are retrieved. More levels can be specfied in a nested fashion under `expand()`.

Query Example: Predicates saved to a variable and queried with `expand()`.
{{< runnable >}}
{
  var(func: eq(name, "Lost in Translation")) {
    pred as _predicate_
    # expand(_all_) { expand(_all_)}
  }

  director(func: eq(name, "Lost in Translation")) {
    name@.
    expand(var(pred)) {
      expand(_all_) {
        name@.
        _uid_
      }
    }
  }
}
{{< /runnable >}}

`_predicate_` returns string valued predicates as a name without language tag.  If the predicate has no string without a language tag, `expand()` won't expand it (see [language preference]({{< relref "#language-support" >}})).  For example, above `name` generally doesn't have strings without tags in the dataset, so `name@.` is required.

## Cascade Directive

With the `@cascade` directive, nodes that don't have all predicates specified in the query are removed. This can be useful in cases where some filter was applied or if nodes might not have all listed predicates.


Query Example: Harry Potter movies, with each actor and characters played.  With `@cascade`, any character not played by an actor called Warwick is removed, as is any Harry Potter movie without any actors called Warwick.  Without `@cascade`, every character is returned, but only those played by actors called Warwick also have the actor name.
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

## Normalize directive

With the `@normalize` directive, only aliased predicates are returned and the result is flattened to remove nesting.

Query Example: Film name, country and first two actors (by UID order) of every Steven Spielberg movie, without `initial_release_date` because no alias is given and flattened by `@normalize`
{{< runnable >}}
{
  director(func:allofterms(name@en, "steven spielberg")) @normalize {
    director: name@en
    director.film {
      film: name@en
      initial_release_date
      starring(first: 2) {
        performance.actor {
          actor: name@en
        }
        performance.character {
          character: name@en
        }
      }
      country {
        country: name@en
      }
    }
  }
}
{{< /runnable >}}

## Debug

For the purposes of debugging, you can attach a query parameter `debug=true` to a query. Attaching this parameter lets you retrieve the `_uid_` attribute for all the entities along with the `server_latency` information.

Query with debug as a query parameter
```
curl "http://localhost:8080/query?debug=true" -XPOST -d $'{
  tbl(func: allofterms(name, "The Big Lebowski")) {
    name@en
  }
}' | python -m json.tool | less
```

Returns `_uid_` and `server_latency`
```
{
  "tbl": [
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


## Schema

For each predicate, the schema specifies the target's type.  If a predicate `p` has type `T`, then for all subject-predicate-object triples `s p o` the object `o` is of schema type `T`.

* On mutations, scalar types are checked and an error thrown if the value cannot be converted to the schema type.

* On query, value results are returned according to the schema type of the predicate.

If a schema type isn't specified before a mutation adds triples for a predicate, then the type is inferred from the first mutation.  This type is either:

* type `uid`, if the first mutation for the predicate has nodes for the subject and object, or

* derived from the [rdf type]({{< relref "#rdf-types" >}}), if the object is a literal and an rdf type is present in the first mutation, or

* `default` type, otherwise.


### Schema Types

Dgraph supports scalar types and the UID type.

#### Scalar Types

For all triples with a predicate of scalar types the object is a literal.

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

Dgraph supports a number of [RDF types in mutations]({{< relref "#language-and-rdf-types" >}}).

As well as implying a schema type for a [first mutation]({{< relref "#schema" >}}), an RDF type can override a schema type for storage.

If a predicate has a schema type and a mutation has an RDF type with a different underlying Dgraph type, the convertibility to schema type is checked, and an error is thrown if they are incompatible, but the value is stored in the RDF type's corresponding Dgraph type.  Query results are always returned in schema type.

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

* sets the schema type to `int`, as implied by the first triple,  
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
    <0x123> <name> "Password Example"
    <0x123> <password> "ThePassword"^^<pwd:password>     .
  }
}
```

to check a password:
```
{
  check(id: 0x123) {
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

<!--
#### Date Time Indices

**to be added after [issue #971](https://github.com/dgraph-io/dgraph/issues/971)**
-->

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

#### Count index

For predicates with the `@count` Dgraph indexes the number of edges out of each node.  The enables fast queries of the form:
```
{
  q(func: gt(count(pred), threshold)) {
    ...
  }
}
```

### Adding or Modifying Schema

Schema mutations add or modify schema.

An index is specified with `@index`, with arguments to specify the tokenizer.  For example:

```
mutation {
  schema {
    name: string @index(exact, fulltext) @count .
    age: int @index .
    friend: uid @count .
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

Adding or removing data in Dgraph is called a mutation.

A mutation that adds triples, does so with the `set` keyword.
```
mutation {
  set {
    # triples in here
  }
}
```

### Triples

The input language is triples in the W3C standard [RDF N-Quad format](https://www.w3.org/TR/n-quads/).

Each triple has the form
```
<subject> <predicate> <object> .
```
Meaning that the graph node identified by `subject` is linked to `object` with directed edge `predicate`.  Each triple ends with a full stop.  The subject of a triple is always a node in the graph, while the object may be a node or a value (a literal).

For example, the triple
```
<0x01> <name> "Alice" .
```
Represents that graph node with ID `0x01` has a `name` with string value `"Alice"`.  While triple
```
<0x01> <friend> <0x02> .
```
Represents that graph node with ID `0x01` is linked with the `friend` edge to node `0x02`.

Dgraph creates a unique 64 bit identifier for every node in the graph - the node's UID.  A mutation either lets Dgraph create the UID as the identifier for the subject or object, using blank or external id nodes, or specifies a known UID from a previous mutation..


### Blank Nodes and UID

Blank nodes in mutations, written `_:identifier`, identify nodes within a mutation.  Dgraph creates a UID identifying each blank node and returns the created UIDs as the mutation result.  For example, mutation:

```
mutation {
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
  "code": "Success",
  "message": "Done",
  "uids": {
    "class": "0x6bc818dc89e78754",
    "x": "0xc3bcc578868b719d",
    "y": "0xb294fb8464357b0a"
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
mutation {
 set {
    <0x6bc818dc89e78754> <student> _:x .
    _:x <name> "Chris" .
 }
}
```

A query can also directly use UID.
```
{
 class(id: 0x6bc818dc89e78754) {
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

### External IDs

Dgraph's input language, RDF, also supports triples of the form `<a_fixed_identifier> <predicate> literal/node` and variants on this, where the label `a_fixed_identifier` is intended as a unique identifier for a node.  For example, mixing [schema.org](http://schema.org) identifiers, [the movie database](https://www.themoviedb.org/) identifiers and blank nodes:

```
_:userA <http://schema.org/type> <http://schema.org/Person> .
_:userA <http://schema.org/name> "FirstName LastName" .
<https://www.themoviedb.org/person/32-robin-wright> <http://schema.org/type> <http://schema.org/Person> .
<https://www.themoviedb.org/person/32-robin-wright> <http://schema.org/name> "Robin Wright" .
```

As of version 0.8 Dgraph doesn't natively support such external IDs as node identifiers.  Instead, external IDs can be stored as properties of a node with an `_xid_` edge.  For example, from the above, the predicate names are valid in Dgraph, but the node identified with `<http://schema.org/Person>` could be identified in Dgraph with a UID, say `0x123`, and an edge

```
<0x123> <_xid_> "http://schema.org/Person" .
```

While Robin Wright might get UID `0x321` and triples

```
<0x321> <_xid_> "https://www.themoviedb.org/person/32-robin-wright" .
<0x321> <http://schema.org/type> <0x123> .
<0x321> <http://schema.org/name> "Robin Wright" .
```

An appropriate schema might be as follows.
```
mutation {
  schema {
    _xid_: string @index(exact) .
    <http://schema.org/type>: uid @reverse .
  }
}
```

Query Example: All people.

```
{
  var(func: eq(_xid_, "http://schema.org/Person")) {
    allPeople as <~http://schema.org/type>
  }

  q(id: var(allPeople)) {
    <http://schema.org/name>
  }
}
```

Query Example: Robin Wright by external ID.

```
{
  robin(func: eq(_xid_, "https://www.themoviedb.org/person/32-robin-wright")) {
    expand(_all_) { expand(_all_) }
  }
}

```

{{% notice "note" %}} `_xid_` edges are not added automatically in mutations.  In general it is a user's responsibility to check for existing `_xid_`'s and add nodes and `_xid_` edges if necessary.  `dgraphloader` adds `_xid_` edges for bulk uploads with `-x`, see [Bulk Data Loading]({{< relref "deploy/index.md#bulk-data-loading" >}}).  Dgraph leaves all checking of uniqueness of such `_xid_`'s to external processes. {{% /notice %}}



### Language and RDF Types

RDF N-Quad allows specifying a language for string values and an RDF type.  Languages are written using `@lang`. For example
```
<0x01> <name> "Adelaide"@en .
<0x01> <name> "Аделаида"@ru .
<0x01> <name> "Adélaïde"@fr .
```
See also [how language is handled in query]({{< relref "#language-support" >}}).

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
| &#60;http&#58;//www.w3.org/2001/XMLSchema#string&#62;   | `string`         |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#dateTime&#62; | `dateTime`       |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#date&#62;     | `dateTime`       |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#int&#62;      | `int`            |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#boolean&#62;  | `bool`           |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#double&#62;   | `float`          |
| &#60;http&#58;//www.w3.org/2001/XMLSchema#float&#62;    | `float`          |


See the section on [RDF schema types]({{< relref "#rdf-types" >}}) to understand how RDF types affect mutations and storage.


### Batch mutations

Each mutation may contain multiple RDF triples.  For large data uploads many such mutations can be batched in parallel.  The tool `dgraphloader` does just this; by default batching 1000 RDF lines into a query, while running 100 such queries in parallel.

Dgraphloader takes as input gzipped N-Quad files (that is triple lists without `mutation { set {`) and batches mutations for all triples in the input.  The tool has documentation of options.

```
dgraphloader --help
```
See also [Bulk Data Loading]({{< relref "deploy/index.md#bulk-data-loading" >}}).

### Delete

A delete mutation, signified with the `delete` keyword, removes triples from the store.  

For example, if the store contained
```
<0xf11168064b01135b> <name> "Lewis Carrol"
<0xf11168064b01135b> <died> "1998"
```

Then delete mutation

```
mutation {
  delete {
     <0xf11168064b01135b> <died> "1998" .
  }
}
```

Deletes the erroneous data and removes it from indexes if present.

For a particular node `N`, all data for predicate `P` (and corresponding indexing) is removed with the pattern `S P *`.  

```
mutation {
  delete {
     <0xf11168064b01135b> <author.of> * .
  }
}
```

The pattern `S * *` deletes all edges out of a node (the node itself may remain as the target of edges), any reverse edges corresponding to the removed edges and any indexing for the removed data.
```
mutation {
  delete {
     <0xf11168064b01135b> * * .
  }
}
```

The pattern `* P *` removes all data for predicate `P`, data for the reverse edge if present and deletes any indexes created on `P`.

```
mutation {
  delete {
     * <author.of> * .
  }
}
```

After such a delete mutation, the schema of the predicate may be changed --- even from UID to scalar, or scalar to UID; such a change is allowed only after all data is deleted.  


### Variables in mutations

A mutation may depend on a query through query variables.  

For example, in a graph with people and ages, the following updates all people 18 and over as adults.

```
{
  adults as var(func: ge(age, 18))
}
mutation {
  set {
    var(adults) <isadult> "true"^^<xs:boolean> .
  }
}
```

Variables are also allowed in delete mutations.  The following removes any data about electoral role for minors.

```
{
  minors as var(func: lt(age, 18))
}
mutation {
  set {
    var(minors) <electoral_registration> * .
  }
}
```

Internally, such mutations are are expanded to a triple per UID in the variable.  Hence mutations with variables on both sides of the predicate `var(variable1) <edge> var(variable2)` are expanded to the cross product.


## Facets : Edge attributes

Dgraph supports facets --- **key value pairs on edges** --- as an extension to RDF triples. That is, facets add properties to edges, rather than to nodes.
For example, a `friend` edge between two nodes may have a boolean property of `close` friendship.
Facets can also be used as `weights` for edges.

Though you may find yourself leaning towards facets many times, they should not be misused.  It wouldn't be correct modeling to give the `friend` edge a facet `date_of_birth`. That should be an edge for the friend.  However, a facet like `start_of_friendship` might be appropriate.  Facets are however not first class citizen in Dgraph like predicates.

Facet keys are strings and values can be `string`, `bool`, `int`, `float` and `dateTime`.
For `int` and `float`, only decimal integers upto 32 signed bits, and 64 bit float values are accepted respectively.

The following mutation is used throughout this section on facets.  The mutation adds data for some peoples and, for example, records a `since` facet in `mobile` and `car` to record when Alice bought the car and started using the mobile number.

```
curl localhost:8080/query -XPOST -d $'
mutation {
  schema {
    name: string @index(exact, term) .
    rated: uid @reverse @count .
  }
  set {

    # -- Facets on scalar predicates
    _:alice <name> "Alice" .
    _:alice <mobile> "040123456" (since=2006-01-02T15:04:05) .
    _:alice <car> "MA0123" (since=2006-02-02T13:01:09, first=true) .

    _:bob <name> "Bob" .
    _:bob <car> "MA0134" (since=2006-02-02T13:01:09) .

    _:charlie <name> "Charlie" .
    _:dave <name> "Dave" .


    # -- Facets on UID predicates
    _:alice <friend> _:bob (close=true, relative=false) .
    _:alice <friend> _:charlie (close=false, relative=true) .
    _:alice <friend> _:dave (close=true, relative=true) .


    # -- Facets for variable propagation
    _:movie1 <name> "Movie 1" .
    _:movie2 <name> "Movie 2" .
    _:movie3 <name> "Movie 3" .

    _:alice <rated> _:movie1 (rating=3) .
    _:alice <rated> _:movie2 (rating=2) .
    _:alice <rated> _:movie3 (rating=5) .

    _:bob <rated> _:movie1 (rating=5) .
    _:bob <rated> _:movie2 (rating=5) .
    _:bob <rated> _:movie3 (rating=5) .

    _:charlie <rated> _:movie1 (rating=2) .
    _:charlie <rated> _:movie2 (rating=5) .
    _:charlie <rated> _:movie3 (rating=1) .
  }
}' | python -m json.tool | less
```

### Facets on scalar predicates


Querying `name`, `mobile` and `car` of Alice gives the same result as without facets.

```
curl localhost:8080/query -XPOST -d $'{
  data(func: eq(name, "Alice")) {
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
            "name": "Alice"
        }
    ]
}
```


The syntax `@facets(facet-name)` is used to query facet data. For Alice the `since` facet for `mobile` and `car` are queried as follows.

```
curl localhost:8080/query -XPOST -d $'{
  data(func: eq(name, "Alice")) {
     name
     mobile @facets(since)
     car @facets(since)
  }
}' | python -m json.tool | less
```

Facets are retuned at the same level as the corresponding edge and have keys of edge and then facet name.  The response from the query above is:

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
            "name": "Alice"
        }
    ]
}

```

All facets on an edge are queried with `@facets`.

```
curl localhost:8080/query -XPOST -d $'{
  data(func: eq(name, "Alice")) {
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
            "name": "Alice"
        }
    ]
}
```


### Facets on UID predicates

Facets on UID edges work similarly to facets on value edges.

For example, `friend` is an edge with facet `close`.
It was set to true for friendship between Alice and Bob
and false for friendship between Alice and Charlie.

A query for friends of Alice.
```
curl localhost:8080/query -XPOST -d $'
{
  data(func: eq(name, "Alice")) {
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
                    "name": "Bob"
                },
                {
                    "name": "Charlie"
                },
                {
                    "name": "Dave"
                }
            ],
            "name": "Alice"
        }
    ]
}
```

A query for friends and the facet `close` with `@facets(close)`.

```
curl localhost:8080/query -XPOST -d $'{
   data(func: eq(name, "Alice")) {
     name
     friend @facets(close) {
       name
     }
   }
}' | python -m json.tool | less
```

As with facets on value edges, the result contains key `@facets` in each child of `friend`.
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
                    "name": "Bob"
                },
                {
                    "@facets": {
                        "_": {
                            "close": false
                        }
                    },
                    "name": "Charlie"
                },
                {
                    "@facets": {
                        "_": {
                            "close": true
                        }
                    },
                    "name": "Dave"
                }
            ],
            "name": "Alice"
        }
    ]
}
```

For uid edges like `friend`, facets go to the corresponding child's `@facets` under key `_`.  Hence, the facets can be distinguished when the output contains both facets on uid-edges (like `friend`) and value-edges (like `car`).

```
curl localhost:8081/query -XPOST -d $'{
  data(func: eq(name, "Alice")) {
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
                            "close": true,
                            "relative": false
                        },
                        "car": {
                            "since": "2006-02-02T13:01:09Z"
                        }
                    },
                    "car": "MA0134",
                    "name": "Bob"
                },
                {
                    "@facets": {
                        "_": {
                            "close": false,
                            "relative": true
                        }
                    },
                    "name": "Charlie"
                },
                {
                    "@facets": {
                        "_": {
                            "close": true,
                            "relative": true
                        }
                    },
                    "name": "Dave"
                }
            ],
            "name": "Alice"
        }
    ]
}
```

Bob has a `car` and it has a facet `since`, which, in the results, is part of the same object as Bob.
Also, the `close` relationship between Bob and Alice is part of Bob's output object.
Charlie does not have `car` edge and thus only UID facets.

### Filtering on facets

Dgraph supports filtering edges based on facets.
Filtering works similarly to how it works on edges without facets and has the same available functions.


Find Alice's close friends
```
curl localhost:8080/query -XPOST -d $'{
  data(func: eq(name, "Alice")) {
    friend @facets(eq(close, true)) {
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
                    "name": "Bob"
                },
                {
                    "name": "Dave"
                }
            ]
        }
    ]
}
```

To return facets as well as filter, add another `@facets(<facetname>)` to the query.

```
curl localhost:8080/query -XPOST -d $'{
  data(func: eq(name, "Alice")) {
    friend @facets(eq(close, true)) @facets(relative) { # filter close friends and give relative status
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
                    "@facets": {
                        "_": {
                            "relative": false
                        }
                    },
                    "name": "Bob"
                },
                {
                    "@facets": {
                        "_": {
                            "relative": true
                        }
                    },
                    "name": "Dave"
                }
            ]
        }
    ]
}
```

Facet queries can be composed with `AND`, `OR` and `NOT`.

```
curl localhost:8080/query -XPOST -d $'{
  data(func: eq(name, "Alice")) {
    friend @facets(eq(close, true) AND eq(relative, true)) @facets(relative) { # filter close friends in my relation
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
                    "@facets": {
                        "_": {
                            "relative": true
                        }
                    },
                    "name": "Dave"
                }
            ]
        }
    ]
}
```


### Assigning Facet values to a variable

Facets on UID edges can be stored in [value variables]({{< relref "#value-variables" >}}).  The variable is a map from the edge target to the facet value.

Alice's friends reported by variables for `close` and `relative`.
```
curl localhost:8081/query -XPOST -d $'{
  var(func: eq(name, "Alice")) {
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
            "name": "Bob",
            "var(a)": true
        },
        {
            "name": "Charlie",
            "var(a)": false
        },
        {
            "name": "Dave",
            "var(a)": true
        }
    ],
    "relative": [
        {
            "name": "Bob",
            "var(b)": false
        },
        {
            "name": "Charlie",
            "var(b)": true
        },
        {
            "name": "Dave",
            "var(b)": true
        }
    ]
}
```

### Facets and Variable Propagation

Facet values of `int` and `float` can be assigned to variables and thus the [values propagate]({{< relref "#variable-propagation" >}}).


Alice, Bob and Charlie each rated every movie.  A value variable on facet `rating` maps movies to ratings.  A query that reaches a movie through multiple paths sums the ratings on each path.  The following sums Alice, Bob and Charlie's ratings for the three movies.

```
curl localhost:8080/query -XPOST -d $'{
  var(func: anyofterms(name, "Alice Bob Charlie")) {
    num_raters as math(1)
    rated @facets(r as rating) {
      total_rating as math(r) # sum of the 3 ratings
      average_rating as math(total_rating / num_raters)
    }
  }
  data(id: var(total_rating)) {
    name
    var(total_rating)
    var(average_rating)
  }

}' | python -m json.tool | less
```

Output
```
{
    "data": [
        {
            "name": "Movie 1",
            "var(average_rating)": 3.333333,
            "var(total_rating)": 10
        },
        {
            "name": "Movie 2",
            "var(average_rating)": 4.0,
            "var(total_rating)": 12
        },
        {
            "name": "Movie 3",
            "var(average_rating)": 3.666667,
            "var(total_rating)": 11
        }
    ]
}
```


### Facets and Aggregation

Facet values assigned to value variables can be aggregated.

```
curl localhost:8080/query -XPOST -d $'{
  data(func: eq(name, "Alice")) {
    name
    rated @facets(r as rating) {
      name
    }
    avg(var(r))
  }
}' | python -m json.tool | less
```

Output:

```
{
    "data": [
        {
            "avg(var(r))": 3.333333,
            "name": "Alice",
            "rated": [
                {
                    "@facets": {
                        "_": {
                            "rating": 3
                        }
                    },
                    "name": "Movie 1"
                },
                {
                    "@facets": {
                        "_": {
                            "rating": 2
                        }
                    },
                    "name": "Movie 2"
                },
                {
                    "@facets": {
                        "_": {
                            "rating": 5
                        }
                    },
                    "name": "Movie 3"
                }
            ]
        }
    ]
}
```

Note though that `r` is a map from movies to the sum of ratings on edges in the query reaching the movie.  Hence, the following does not correctly calculate the average ratings for Alice and Bob individually --- it calculates 2 times the average of both Alice and Bob's ratings.
```
curl localhost:8080/query -XPOST -d $'{
  data(func: anyofterms(name, "Alice Bob")) {
    name
    rated @facets(r as rating) {
      name
    }
    avg(var(r))
  }
}' | python -m json.tool | less
```

Output:
```
{
    "data": [
        {
            "avg(var(r))": 8.333333,
            "name": "Alice",
            "rated": [
                {
                    "@facets": {
                        "_": {
                            "rating": 3
                        }
                    },
                    "name": "Movie 1"
                },
                {
                    "@facets": {
                        "_": {
                            "rating": 2
                        }
                    },
                    "name": "Movie 2"
                },
                {
                    "@facets": {
                        "_": {
                            "rating": 5
                        }
                    },
                    "name": "Movie 3"
                }
            ]
        },
        {
            "avg(var(r))": 8.333333,
            "name": "Bob",
            "rated": [
                {
                    "@facets": {
                        "_": {
                            "rating": 5
                        }
                    },
                    "name": "Movie 1"
                },
                {
                    "@facets": {
                        "_": {
                            "rating": 5
                        }
                    },
                    "name": "Movie 2"
                },
                {
                    "@facets": {
                        "_": {
                            "rating": 5
                        }
                    },
                    "name": "Movie 3"
                }
            ]
        }
    ]
}
```

Calculating the average ratings of users requires a variable that maps users to the sum of their ratings.

```
curl localhost:8081/query -XPOST -d $'{
  var(func: has(~rated)) {
    num_rated as math(1)
    ~rated @facets(r as rating) {
      avg_rating as math(r / num_rated)
    }
  }

  data(id: var(avg_rating)) {
    name
    var(avg_rating)
  }
}' | python -m json.tool | less
```

Output:
```
{
    "data": [
        {
            "name": "Alice",
            "var(avg_rating)": 3.333333
        },
        {
            "name": "Bob",
            "var(avg_rating)": 5.0
        },
        {
            "name": "Charlie",
            "var(avg_rating)": 2.666667
        }
    ]
}
```

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
