+++
date = "2017-03-20T22:25:17+11:00"
title = "Functions"
weight = 2 
[menu.main]
    parent = "query-language"
+++

Functions allow filtering based on properties of nodes or [variables]({{<relref "query-language/value-variables.md">}}).  Functions can be applied in the query root or in filters.

{{% notice "note" %}}Support for filters on non-indexed predicates was added with Dgraph `v1.2.0`.
{{% /notice %}}

Comparison functions (`eq`, `ge`, `gt`, `le`, `lt`) in the query root (aka `func:`) can only
be applied on [indexed predicates]({{< relref "query-language/schema.md#indexing" >}}). Since v1.2, comparison functions
can now be used on [@filter]({{<relref "query-language/graphql-fundamentals.md#applying-filters" >}}) directives even on predicates
that have not been indexed.
Filtering on non-indexed predicates can be slow for large datasets, as they require
iterating over all of the possible values at the level where the filter is being used.

All other functions, in the query root or in the filter can only be applied to indexed predicates.

For functions on string valued predicates, if no language preference is given, the function is applied to all languages and strings without a language tag; if a language preference is given, the function is applied only to strings of the given language.

## Term matching

### allofterms

Syntax Example: `allofterms(predicate, "space-separated term list")`

Schema Types: `string`

Index Required: `term`


Matches strings that have all specified terms in any order; case insensitive.
#### Usage at root

Query Example: All nodes that have `name` containing terms `indiana` and `jones`, returning the English name and genre in English.

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
#### Usage as Filter

Query Example: All Steven Spielberg films that contain the words `indiana` and `jones`.  The `@filter(has(director.film))` removes nodes with name Steven Spielberg that aren't the director --- the data also contains a character in a film called Steven Spielberg.

{{< runnable >}}
{
  me(func: eq(name@en, "Steven Spielberg")) @filter(has(director.film)) {
    name@en
    director.film @filter(allofterms(name@en, "jones indiana"))  {
      name@en
    }
  }
}
{{< /runnable >}}

### anyofterms


Syntax Example: `anyofterms(predicate, "space-separated term list")`

Schema Types: `string`

Index Required: `term`


Matches strings that have any of the specified terms in any order; case insensitive.
#### Usage at root

Query Example: All nodes that have a `name` containing either `poison` or `peacock`.  Many of the returned nodes are movies, but people like Joan Peacock also meet the search terms because without a [cascade directive]({{< relref "query-language/cascade-directive.md">}}) the query doesn't require a genre.

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

#### Usage as filter

Query Example: All Steven Spielberg movies that contain `war` or `spies`.  The `@filter(has(director.film))` removes nodes with name Steven Spielberg that aren't the director --- the data also contains a character in a film called Steven Spielberg.

{{< runnable >}}
{
  me(func: eq(name@en, "Steven Spielberg")) @filter(has(director.film)) {
    name@en
    director.film @filter(anyofterms(name@en, "war spies"))  {
      name@en
    }
  }
}
{{< /runnable >}}

## Regular Expressions


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

### Technical details

A Trigram is a substring of three continuous runes. For example, `Dgraph` has trigrams `Dgr`, `gra`, `rap`, `aph`.

To ensure efficiency of regular expression matching, Dgraph uses [trigram indexing](https://swtch.com/~rsc/regexp/regexp4.html).  That is, Dgraph converts the regular expression to a trigram query, uses the trigram index and trigram query to find possible matches and applies the full regular expression search only to the possibles.
### Writing Efficient Regular Expressions and Limitations

Keep the following in mind when designing regular expression queries.

- At least one trigram must be matched by the regular expression (patterns shorter than 3 runes are not supported).  That is, Dgraph requires regular expressions that can be converted to a trigram query.
- The number of alternative trigrams matched by the regular expression should be as small as possible  (`[a-zA-Z][a-zA-Z][0-9]` is not a good idea).  Many possible matches means the full regular expression is checked against many strings; where as, if the expression enforces more trigrams to match, Dgraph can make better use of the index and check the full regular expression against a smaller set of possible matches.
- Thus, the regular expression should be as precise as possible.  Matching longer strings means more required trigrams, which helps to effectively use the index.
- If repeat specifications (`*`, `+`, `?`, `{n,m}`) are used, the entire regular expression must not match the _empty_ string or _any_ string: for example, `*` may be used like `[Aa]bcd*` but not like `(abcd)*` or `(abcd)|((defg)*)`
- Repeat specifications after bracket expressions (e.g. `[fgh]{7}`, `[0-9]+` or `[a-z]{3,5}`) are often considered as matching any string because they match too many trigrams.
- If the partial result (for subset of trigrams) exceeds 1000000 uids during index scan, the query is stopped to prohibit expensive queries.

## Fuzzy matching


Syntax: `match(predicate, string, distance)`

Schema Types: `string`

Index Required: `trigram`

Matches predicate values by calculating the [Levenshtein distance](https://en.wikipedia.org/wiki/Levenshtein_distance) to the string,
also known as _fuzzy matching_. The distance parameter must be greater than zero (0). Using a greater distance value can yield more but less accurate results.

Query Example: At root, fuzzy match nodes similar to `Stephen`, with a distance value of less than or equal to 8.

{{< runnable >}}
{
  directors(func: match(name@en, Stephen, 8)) {
    name@en
  }
}
{{< /runnable >}}

Same query with a Levenshtein distance of 3.

{{< runnable >}}
{
  directors(func: match(name@en, Stephen, 3)) {
    name@en
  }
}
{{< /runnable >}}

## Full-Text Search

Syntax Examples: `alloftext(predicate, "space-separated text")` and `anyoftext(predicate, "space-separated text")`

Schema Types: `string`

Index Required: `fulltext`


Apply full-text search with stemming and stop words to find strings matching all or any of the given text.

The following steps are applied during index generation and to process full-text search arguments:

1. Tokenization (according to Unicode word boundaries).
1. Conversion to lowercase.
1. Unicode-normalization (to [Normalization Form KC](http://unicode.org/reports/tr15/#Norm_Forms)).
1. Stemming using language-specific stemmer (if supported by language).
1. Stop words removal (if supported by language).

Dgraph uses [bleve](https://github.com/blevesearch/bleve) for its full-text search indexing. See also the bleve language specific [stop word lists](https://github.com/blevesearch/bleve/tree/master/analysis/lang).

Following table contains all supported languages, corresponding country-codes, stemming and stop words filtering support.

|  Language  | Country Code | Stemming | Stop words |
| :--------: | :----------: | :------: | :--------: |
|   Arabic   |      ar      | &#10003; |  &#10003;  |
|  Armenian  |      hy      |          |  &#10003;  |
|   Basque   |      eu      |          |  &#10003;  |
| Bulgarian  |      bg      |          |  &#10003;  |
|  Catalan   |      ca      |          |  &#10003;  |
|  Chinese   |      zh      | &#10003; |  &#10003;  |
|   Czech    |      cs      |          |  &#10003;  |
|   Danish   |      da      | &#10003; |  &#10003;  |
|   Dutch    |      nl      | &#10003; |  &#10003;  |
|  English   |      en      | &#10003; |  &#10003;  |
|  Finnish   |      fi      | &#10003; |  &#10003;  |
|   French   |      fr      | &#10003; |  &#10003;  |
|   Gaelic   |      ga      |          |  &#10003;  |
|  Galician  |      gl      |          |  &#10003;  |
|   German   |      de      | &#10003; |  &#10003;  |
|   Greek    |      el      |          |  &#10003;  |
|   Hindi    |      hi      | &#10003; |  &#10003;  |
| Hungarian  |      hu      | &#10003; |  &#10003;  |
| Indonesian |      id      |          |  &#10003;  |
|  Italian   |      it      | &#10003; |  &#10003;  |
|  Japanese  |      ja      | &#10003; |  &#10003;  |
|   Korean   |      ko      | &#10003; |  &#10003;  |
| Norwegian  |      no      | &#10003; |  &#10003;  |
|  Persian   |      fa      |          |  &#10003;  |
| Portuguese |      pt      | &#10003; |  &#10003;  |
|  Romanian  |      ro      | &#10003; |  &#10003;  |
|  Russian   |      ru      | &#10003; |  &#10003;  |
|  Spanish   |      es      | &#10003; |  &#10003;  |
|  Swedish   |      sv      | &#10003; |  &#10003;  |
|  Turkish   |      tr      | &#10003; |  &#10003;  |


Query Example: All names that have `dog`, `dogs`, `bark`, `barks`, `barking`, etc.  Stop word removal eliminates `the` and `which`.

{{< runnable >}}
{
  movie(func:alloftext(name@en, "the dog which barks")) {
    name@en
  }
}
{{< /runnable >}}

## Inequality
### equal to

Syntax Examples:

* `eq(predicate, value)`
* `eq(val(varName), value)`
* `eq(predicate, val(varName))`
* `eq(count(predicate), value)`
* `eq(predicate, [val1, val2, ..., valN])`
* `eq(predicate, [$var1, "value", ..., $varN])`

Schema Types: `int`, `float`, `bool`, `string`, `dateTime`

Index Required: An index is required for the `eq(predicate, ...)` forms (see table below) when used at query root.  For `count(predicate)` at the query root, the `@count` index is required. For variables the values have been calculated as part of the query, so no index is required.

| Type       | Index Options |
|:-----------|:--------------|
| `int`      | `int`         |
| `float`    | `float`       |
| `bool`     | `bool`        |
| `string`   | `exact`, `hash` |
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

  stevens(func: uid(steve)) @filter(eq(val(films), [1,2,3])) {
    name@en
    numFilms : val(films)
  }
}
{{< /runnable >}}

### less than, less than or equal to, greater than and greater than or equal to

Syntax Examples: for inequality `IE`

* `IE(predicate, value)`
* `IE(val(varName), value)`
* `IE(predicate, val(varName))`
* `IE(count(predicate), value)`

With `IE` replaced by

* `le` less than or equal to
* `lt` less than
* `ge` greater than or equal to
* `gt` greather than

Schema Types: `int`, `float`, `string`, `dateTime`

Index required: An index is required for the `IE(predicate, ...)` forms (see table below) when used at query root.  For `count(predicate)` at the query root, the `@count` index is required. For variables the values have been calculated as part of the query, so no index is required.

| Type       | Index Options |
|:-----------|:--------------|
| `int`      | `int`         |
| `float`    | `float`       |
| `string`   | `exact`       |
| `dateTime` | `dateTime`    |


Query Example: Ridley Scott movies released before 1980.

{{< runnable >}}
{
  me(func: eq(name@en, "Ridley Scott")) {
    name@en
    director.film @filter(lt(initial_release_date, "1980-01-01"))  {
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
    total as sum(val(num_actors))
  }

  dirs(func: uid(ID)) @filter(gt(val(total), 100)) {
    name@en
    total_actors : val(total)
  }
}
{{< /runnable >}}



Query Example: A movie in each genre that has over 30000 movies.  Because there is no order specified on genres, the order will be by UID.  The [count index]({{< relref "query-language/schema.md#count-index">}}) records the number of edges out of nodes and makes such queries more .

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

Query Example: Directors called Steven and their movies which have `initial_release_date` greater
than that of the movie Minority Report.

{{< runnable >}}
{
  var(func: eq(name@en,"Minority Report")) {
    d as initial_release_date
  }

  me(func: eq(name@en, "Steven Spielberg")) {
    name@en
    director.film @filter(ge(initial_release_date, val(d))) {
      initial_release_date
      name@en
    }
  }
}
{{< /runnable >}}

## uid

Syntax Examples:

* `q(func: uid(<uid>)) `
* `predicate @filter(uid(<uid1>, ..., <uidn>))`
* `predicate @filter(uid(a))` for variable `a`
* `q(func: uid(a,b))` for variables `a` and `b`


Filters nodes at the current query level to only nodes in the given set of UIDs.

For query variable `a`, `uid(a)` represents the set of UIDs stored in `a`.  For value variable `b`, `uid(b)` represents the UIDs from the UID to value map.  With two or more variables, `uid(a,b,...)` represents the union of all the variables.

`uid(<uid>)`, like an identity function, will return the requested UID even if the node does not have any edges.

Query Example: If the UID of a node is known, values for the node can be read directly.  The films of Priyanka Chopra by known UID

{{< runnable >}}
{
  films(func: uid(0x2c964)) {
    name@hi
    actor.film {
      performance.film {
        name@hi
      }
    }
  }
}
{{< /runnable >}}



Query Example: The films of Taraji Henson by genre.
{{< runnable >}}
{
  var(func: allofterms(name@en, "Taraji Henson")) {
    actor.film {
      F as performance.film {
        G as genre
      }
    }
  }

  Taraji_films_by_genre(func: uid(G)) {
    genre_name : name@en
    films : ~genre @filter(uid(F)) {
      film_name : name@en
    }
  }
}
{{< /runnable >}}



Query Example: Taraji Henson films ordered by number of genres, with genres listed in order of how many films Taraji has made in each genre.
{{< runnable >}}
{
  var(func: allofterms(name@en, "Taraji Henson")) {
    actor.film {
      F as performance.film {
        G as count(genre)
        genre {
          C as count(~genre @filter(uid(F)))
        }
      }
    }
  }

  Taraji_films_by_genre_count(func: uid(G), orderdesc: val(G)) {
    film_name : name@en
    genres : genre (orderdesc: val(C)) {
      genre_name : name@en
    }
  }
}
{{< /runnable >}}

## uid_in


Syntax Examples:

* `q(func: ...) @filter(uid_in(predicate, <uid>))`
* `predicate1 @filter(uid_in(predicate2, <uid>))`

Schema Types: UID

Index Required: none

While the `uid` function filters nodes at the current level based on UID, function `uid_in` allows looking ahead along an edge to check that it leads to a particular UID.  This can often save an extra query block and avoids returning the edge.

`uid_in` cannot be used at root, it accepts one UID constant as its argument (not a variable).


Query Example: The collaborations of Marc Caro and Jean-Pierre Jeunet (UID 0x99706).  If the UID of Jean-Pierre Jeunet is known, querying this way removes the need to have a block extracting his UID into a variable and the extra edge traversal and filter for `~director.film`.
{{< runnable >}}
{
  caro(func: eq(name@en, "Marc Caro")) {
    name@en
    director.film @filter(uid_in(~director.film, 0x99706)) {
      name@en
    }
  }
}
{{< /runnable >}}

## has

Syntax Examples: `has(predicate)`

Schema Types: all

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
## Geolocation

{{% notice "note" %}} As of now we only support indexing Point, Polygon and MultiPolygon [geometry types](https://github.com/twpayne/go-geom#geometry-types). However, Dgraph can store other types of gelocation data. {{% /notice %}}

Note that for geo queries, any polygon with holes is replace with the outer loop, ignoring holes.  Also, as for version 0.7.7 polygon containment checks are approximate.
### Mutations

To make use of the geo functions you would need an index on your predicate.
```
loc: geo @index(geo) .
```

Here is how you would add a `Point`.

```
{
  set {
    <_:0xeb1dde9c> <loc> "{'type':'Point','coordinates':[-122.4220186,37.772318]}"^^<geo:geojson> .
    <_:0xeb1dde9c> <name> "Hamon Tower" .
    <_:0xeb1dde9c> <dgraph.type> "Location" .
  }
}
```

Here is how you would associate a `Polygon` with a node. Adding a `MultiPolygon` is also similar.

```
{
  set {
    <_:0xf76c276b> <loc> "{'type':'Polygon','coordinates':[[[-122.409869,37.7785442],[-122.4097444,37.7786443],[-122.4097544,37.7786521],[-122.4096334,37.7787494],[-122.4096233,37.7787416],[-122.4094004,37.7789207],[-122.4095818,37.7790617],[-122.4097883,37.7792189],[-122.4102599,37.7788413],[-122.409869,37.7785442]],[[-122.4097357,37.7787848],[-122.4098499,37.778693],[-122.4099025,37.7787339],[-122.4097882,37.7788257],[-122.4097357,37.7787848]]]}"^^<geo:geojson> .
    <_:0xf76c276b> <name> "Best Western Americana Hotel" .
    <_:0xf76c276b> <dgraph.type> "Location" .
  }
}
```

The above examples have been picked from our [SF Tourism](https://github.com/dgraph-io/benchmarks/blob/master/data/sf.tourism.gz?raw=true) dataset.
### Query
#### near

Syntax Example: `near(predicate, [long, lat], distance)`

Schema Types: `geo`

Index Required: `geo`

Matches all entities where the location given by `predicate` is within `distance` meters of geojson coordinate `[long, lat]`.

Query Example: Tourist destinations within 1000 meters (1 kilometer) of a point in Golden Gate Park in San Francisco.

{{< runnable >}}
{
  tourist(func: near(loc, [-122.469829, 37.771935], 1000) ) {
    name
  }
}
{{< /runnable >}}

#### within

Syntax Example: `within(predicate, [[[long1, lat1], ..., [longN, latN]]])`

Schema Types: `geo`

Index Required: `geo`

Matches all entities where the location given by `predicate` lies within the polygon specified by the geojson coordinate array.

Query Example: Tourist destinations within the specified area of Golden Gate Park, San Francisco.

{{< runnable >}}
{
  tourist(func: within(loc, [[[-122.47266769409178, 37.769018558337926 ], [ -122.47266769409178, 37.773699921075135 ], [ -122.4651575088501, 37.773699921075135 ], [ -122.4651575088501, 37.769018558337926 ], [ -122.47266769409178, 37.769018558337926]]] )) {
    name
  }
}
{{< /runnable >}}

#### contains

Syntax Examples: `contains(predicate, [long, lat])` or `contains(predicate, [[long1, lat1], ..., [longN, latN]])`

Schema Types: `geo`

Index Required: `geo`

Matches all entities where the polygon describing the location given by `predicate` contains geojson coordinate `[long, lat]` or given geojson polygon.

Query Example : All entities that contain a point in the flamingo enclosure of San Francisco Zoo.
{{< runnable >}}
{
  tourist(func: contains(loc, [ -122.50326097011566, 37.73353615592843 ] )) {
    name
  }
}
{{< /runnable >}}

#### intersects

Syntax Example: `intersects(predicate, [[[long1, lat1], ..., [longN, latN]]])`

Schema Types: `geo`

Index Required: `geo`

Matches all entities where the polygon describing the location given by `predicate` intersects the given geojson polygon.


{{< runnable >}}
{
  tourist(func: intersects(loc, [[[-122.503325343132, 37.73345766902749 ], [ -122.503325343132, 37.733903134117966 ], [ -122.50271648168564, 37.733903134117966 ], [ -122.50271648168564, 37.73345766902749 ], [ -122.503325343132, 37.73345766902749]]] )) {
    name
  }
}
{{< /runnable >}}


