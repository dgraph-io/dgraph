+++
date = "2017-03-20T22:25:17+11:00"
title = "GroupBy"
weight = 13
[menu.main]
    parent = "query-language"
+++

Syntax Examples:

* `q(func: ...) @groupby(predicate) { min(...) }`
* `predicate @groupby(pred) { count(uid) }``


A `groupby` query aggregates query results given a set of properties on which to group elements.  For example, a query containing the block `friend @groupby(age) { count(uid) }`, finds all nodes reachable along the friend edge, partitions these into groups based on age, then counts how many nodes are in each group.  The returned result is the grouped edges and the aggregations.

Inside a `groupby` block, only aggregations are allowed and `count` may only be applied to `uid`.

If the `groupby` is applied to a `uid` predicate, the resulting aggregations can be saved in a variable (mapping the grouped UIDs to aggregate values) and used elsewhere in the query to extract information other than the grouped or aggregated edges.

Query Example: For Steven Spielberg movies, count the number of movies in each genre and for each of those genres return the genre name and the count.  The name can't be extracted in the `groupby` because it is not an aggregate, but `uid(a)` can be used to extract the UIDs from the UID to value map and thus organize the `byGenre` query by genre UID.


{{< runnable >}}
{
  var(func:allofterms(name@en, "steven spielberg")) {
    director.film @groupby(genre) {
      a as count(uid)
      # a is a genre UID to count value variable
    }
  }

  byGenre(func: uid(a), orderdesc: val(a)) {
    name@en
    total_movies : val(a)
  }
}
{{< /runnable >}}

Query Example: Actors from Tim Burton movies and how many roles they have played in Tim Burton movies.
{{< runnable >}}
{
  var(func:allofterms(name@en, "Tim Burton")) {
    director.film {
      starring @groupby(performance.actor) {
        a as count(uid)
        # a is an actor UID to count value variable
      }
    }
  }

  byActor(func: uid(a), orderdesc: val(a)) {
    name@en
    val(a)
  }
}
{{< /runnable >}}
