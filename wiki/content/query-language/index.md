+++
title = "Query Language - GraphQL+-"
+++

Dgraph's GraphQL+- is based on Facebook's [GraphQL](https://facebook.github.io/graphql/).  GraphQL wasn't developed for Graph databases, but it's graph-like query syntax, schema validation and subgraph shaped response make it a great language choice.  We've modified the language to better support graph operations, adding and removing features to get the best fit for graph databases.  We're calling this simplified, feature rich language, ''GraphQL+-''.

GraphQL+- is a work in progress. We're adding more features and we might further simplify existing ones.

## Take a Tour - https://tour.dgraph.io

This document is the Dgraph query reference material.  It is not a tutorial.  It's designed as a reference for users who already know how to write queries in GraphQL+- but need to check syntax, or indices, or functions, etc.

{{% notice "note" %}}If you are new to Dgraph and want to learn how to use Dgraph and GraphQL+-, take the tour - https://tour.dgraph.io{{% /notice %}}

### Running examples

The examples in this reference use a database of 21 million triples about movies and actors.  The example queries run and return results.  The queries are executed by an instance of Dgraph running at https://play.dgraph.io/.  To run the queries locally or experiment a bit more, see the Getting Started guide on how to start Dgraph and how to load the 21million and tourism (for location queries) datasets.

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
    name
    initial_release_date
    netflix_id
  }
}
{{< /runnable >}}

The query first searches the graph, using indexes to make the search efficient, for all nodes with a `name` edge equalling "Blade Runner".  For the found node the query then returns the listed outgoing edges.

Every node had a unique identifier.  The `_uid_` edge in the query above returns that identifier.  If the required node is already known, rather than applying a function with `func:`, the node can be found directly with `id:`.  If a node has an XID, it can also be found in the same way.

Query Example: "Blade Runner" movie data found by UID.

{{< runnable >}}
{
  bladerunner(id: 0x3cf6ed367ae4fa80) {
    _uid_
    name
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
    name
    initial_release_date
    netflix_id
  }
}
{{< /runnable >}}


### Expanding Graph Edges

A query expands edges from node to node by nesting query blocks.

Query Example: The actors and characters played in "Blade Runner"
{{< runnable >}}
{
  brCharacters(func: eq(name, "Blade Runner")) {
    name
    initial_release_date
    starring {
      performance.actor {
        name  # actor name
      }
      performance.character {
        name  # character name
      }
    }
  }
}
{{< /runnable >}}


#### Comments

Anything on a line following a `#` is a comment

### Applying Filters

The query root finds an initial set of nodes and the query proceeds by returning values and following edges to further nodes - any node reached in the query is found by traversal after the search at root.  The nodes found can be filtered by applying `@filter`, either after the root or at any edge.

Query Example: "Blade Runner" director Ridley Scott's movies released before the year 2000.
{{< runnable >}}
{
  scott(func: eq(name, "Ridley Scott")) {
    name
    initial_release_date
    director.film @filter(le(initial_release_date, "2000")) {
      name
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
    name
    initial_release_date
    netflix_id
  }
}
{{< /runnable >}}

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
  me(func: allofterms(name, "jones indiana")) {
    name@en
    genre {
      name@en
    }
  }
}
{{< /runnable >}}

##### Usage as Filter

Query Example: Steven Spielberg is XID `m.06pj8`.  All his films that contain the words `indiana` and `jones`.

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


Syntax Example: `anyofterms(predicate, "space-separated term list")`

Schema Types: `string`

Index Required: `term`


Matches strings that have any of the specified terms in any order; case insensitive.

##### Usage at root

Query Example: All nodes that have a `name` containing either `purple` or `peacock`.  Many of the returned nodes are movies, but people like Joan Peacock also meet the search terms because without a [cascade directive]({{< relref "#cascade-directive">}}) the query doesn't require a genre.

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

Query Example: Steven Spielberg is XID `m.06pj8`.  All his movies that contain `war` or `spies`

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

Schema Types: `int`, `float`, `bool`, `string`, `dateTime`

Index Required: An index is required for `eq(predicate, value)` form, but otherwise the values have been calculated as part of the query, so no index is required.

| Type       | Index Options |
|:-----------|:--------------|
| `int`      | `int`         |
| `float`    | `float`       |
| `bool`     | `bool`        |
| `string`   | `exact`, `hash`, `term`, `fulltext` |
| `dateTime` | `dateTime`    |

Note that `eq(count(predicate), value)` iterates through the every `s predicate o` tripple to first generate the count and then applies the filter to the generated counts.  If there are many such triples, this many not be efficient.

Query Example: Movies with exactly two genres.

{{< runnable >}}
{
  me(func: eq(count(genre), 2)) {
    name@en
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

Index required: An index is required for the `IE(predicate, value)` form, but otherwise the values have been calculated as part of the query, so no index is required.

| Type       | Index Options |
|:-----------|:--------------|
| `int`      | `int`         |
| `float`    | `float`       |
| `string`   | `exact`       |
| `dateTime` | `dateTime`    |


Query Example: Steven Spielberg is XID `m.06pj8`.  All his movies released before 1970.

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



Query Example: A movie in each genre that has over 30000 movies.  Because there is no order specified on genres, the order will be by UID.

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

Index Required: no

Determines if a node has a particular predicate.

Query Example: All directors.  Because directors have at least one film, a count at root `func: gt(count(director.film), 0)` would determine all directors, but Dgraph provides `has` as a simpler, faster method.


{{< runnable >}}
{
  all_directors(func: has(director.film)) {
    name@en
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


## Alias

Syntax Examples:

* `aliasName : predicate`
* `aliasName : predicate { ... }`
* `aliasName : varName as ...`
* `aliasName : count(predicate)`
* `aliasName : max(var(varName))`

An alias provides an alternate name in results.  Predicates, variables and aggregates can be aliased by prefixing with the alias name and `:`.  Aliases do not have to be different to the original predicate name, but, within a block, an alias must be distinct from predicate names and other aliases returned in the same block.  Aliases can be used to return the same predicate multiple times within a block.  



Query Example: Directors with `name` matching term `Steven`, their UID, english name, average number of actors per movie, total number of films and the name of each film.  
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
      name : name
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

Query Example: Count of directors who have directed more than five films.

{{< runnable >}}
{
  directors(func: gt(count(director.film), 5)) {
    totalDirectors : count()
  }
}
{{< /runnable >}}


Count can be assigned to a [value variable]({{< relref "#value-variables">}}).

Query Example: The actors of Ang Lee's 'Eat Drink Man Woman' ordered by the number of movies acted in.

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
    name
    director.film(orderasc: initial_release_date) {
      name@en
      name@fr
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

Query Example: All of Angelina Jolie's films, with genres, and Steven Spielberg's films since 2008.

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
   ~genre @filter(var(A) OR var(C)) {  # Movies with either.
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

It is an error to define a value variable but not use it elsewhere in the query.

It therefor only makes sense to use a value variable in a context that matches the same UIDs - if used in a block matching different UIDs the value variable is undefined.

[Facets]({{< relref "#facets-edge-attributes">}}) can be stored in value variables.

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
Here, `A` and `B` are the lists of all UIDs that match these blocks.  Value variable `x` is a mapping from UIDs in `B` to values.  The aggregation `min(var(x))`, however, is computed for each UID in `A`.  That is, it has a semantics of: for each UID in `A`, take the slice of `x` that corresponds to its outgoing `predicateB` edges and compute the aggregation.

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
  director(func: allofterms(name, "steven spielberg")) {
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
| `since`                         | `datetime`                                 | Returns the number of seconds in float from the time specified |
| `pow(a, b)`                     | `int`, `float`                                     | Returns `a to the power b`                                     |
| `logbase(a,b)`                  | `int`, `float`                                     | Returns `log(a)` to the base `b`                               |
| `cond(a, b, c)`                 | first operand must be a boolean                | selects `b` if `a` is true else `c`                            |


Query Example:  Form a score for each of Steven Spielberg's movies as the sum of number of actors, number of genres and number of countries.  List the top five such movies in order of decreasing score.

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

Value variables and aggregations of them can be used in filters.

Query Example: Calculate a score for each Steven Spielberg movie with a condition on release date to penalize movies that are more than 10 years old, filtering on the resulting score.

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


Values calculated with math operations are stored to value variables and so can be aggreated.

Query Example: Compute a score for each Steven Spielberg movie and then aggregate the score.

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
    name
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

































## --- OLD below here ---
## ---- still to be updated ----



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
