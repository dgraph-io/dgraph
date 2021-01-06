+++
date = "2017-03-20T22:25:17+11:00"
title = "Aggregation"
weight = 11
[menu.main]
    parent = "query-language"
+++

Syntax Example: `AG(val(varName))`

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

Aggregation can only be applied to [value variables]({{< relref "query-language/value-variables.md">}}).  An index is not required (the values have already been found and stored in the value variable mapping).

An aggregation is applied at the query block enclosing the variable definition.  As opposed to query variables and value variables, which are global, aggregation is computed locally.  For example:
```
A as predicateA {
  ...
  B as predicateB {
    x as ...some value...
  }
  min(val(x))
}
```
Here, `A` and `B` are the lists of all UIDs that match these blocks.  Value variable `x` is a mapping from UIDs in `B` to values.  The aggregation `min(val(x))`, however, is computed for each UID in `A`.  That is, it has a semantics of: for each UID in `A`, take the slice of `x` that corresponds to `A`'s outgoing `predicateB` edges and compute the aggregation for those values.

Aggregations can themselves be assigned to value variables, making a UID to aggregation map.


## Min

### Usage at Root

Query Example: Get the min initial release date for any Harry Potter movie.

The release date is assigned to a variable, then it is aggregated and fetched in an empty block.
{{< runnable >}}
{
  var(func: allofterms(name@en, "Harry Potter")) {
    d as initial_release_date
  }
  me() {
    min(val(d))
  }
}
{{< /runnable >}}

### Usage at other levels

Query Example:  Directors called Steven and the date of release of their first movie, in ascending order of first movie.

{{< runnable >}}
{
  stevens as var(func: allofterms(name@en, "steven")) {
    director.film {
      ird as initial_release_date
      # ird is a value variable mapping a film UID to its release date
    }
    minIRD as min(val(ird))
    # minIRD is a value variable mapping a director UID to their first release date
  }

  byIRD(func: uid(stevens), orderasc: val(minIRD)) {
    name@en
    firstRelease: val(minIRD)
  }
}
{{< /runnable >}}

## Max

### Usage at Root

Query Example: Get the max initial release date for any Harry Potter movie.

The release date is assigned to a variable, then it is aggregated and fetched in an empty block.
{{< runnable >}}
{
  var(func: allofterms(name@en, "Harry Potter")) {
    d as initial_release_date
  }
  me() {
    max(val(d))
  }
}
{{< /runnable >}}

### Usage at other levels

Query Example: Quentin Tarantino's movies and date of release of the most recent movie.

{{< runnable >}}
{
  director(func: allofterms(name@en, "Quentin Tarantino")) {
    director.film {
      name@en
      x as initial_release_date
    }
    max(val(x))
  }
}
{{< /runnable >}}

## Sum and Avg

### Usage at Root

Query Example: Get the sum and average of number of count of movies directed by people who have
Steven or Tom in their name.

{{< runnable >}}
{
  var(func: anyofterms(name@en, "Steven Tom")) {
    a as count(director.film)
  }

  me() {
    avg(val(a))
    sum(val(a))
  }
}
{{< /runnable >}}

### Usage at other levels

Query Example: Steven Spielberg's movies, with the number of recorded genres per movie, and the total number of genres and average genres per movie.

{{< runnable >}}
{
  director(func: eq(name@en, "Steven Spielberg")) {
    name@en
    director.film {
      name@en
      numGenres : g as count(genre)
    }
    totalGenres : sum(val(g))
    genresPerMovie : avg(val(g))
  }
}
{{< /runnable >}}


## Aggregating Aggregates

Aggregations can be assigned to value variables, and so these variables can in turn be aggregated.

Query Example: For each actor in a Peter Jackson film, find the number of roles played in any movie.  Sum these to find the total number of roles ever played by all actors in the movie.  Then sum the lot to find the total number of roles ever played by actors who have appeared in Peter Jackson movies.  Note that this demonstrates how to aggregate aggregates; the answer in this case isn't quite precise though, because actors that have appeared in multiple Peter Jackson movies are counted more than once.

{{< runnable >}}
{
  PJ as var(func:allofterms(name@en, "Peter Jackson")) {
    director.film {
      starring {  # starring an actor
        performance.actor {
          movies as count(actor.film)
          # number of roles for this actor
        }
        perf_total as sum(val(movies))
      }
      movie_total as sum(val(perf_total))
      # total roles for all actors in this movie
    }
    gt as sum(val(movie_total))
  }

  PJmovies(func: uid(PJ)) {
    name@en
    director.film (orderdesc: val(movie_total), first: 5) {
      name@en
      totalRoles : val(movie_total)
    }
    grandTotal : val(gt)
  }
}
{{< /runnable >}}

