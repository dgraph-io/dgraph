+++
date = "2017-03-20T22:25:17+11:00"
title = "Sorting"
weight = 7
[menu.main]
    parent = "query-language"
+++

Syntax Examples:

* `q(func: ..., orderasc: predicate)`
* `q(func: ..., orderdesc: val(varName))`
* `predicate (orderdesc: predicate) { ... }`
* `predicate @filter(...) (orderasc: N) { ... }`
* `q(func: ..., orderasc: predicate1, orderdesc: predicate2)`

Sortable Types: `int`, `float`, `String`, `dateTime`, `default`

Results can be sorted in ascending order (`orderasc`) or descending order (`orderdesc`) by a predicate or variable.

For sorting on predicates with [sortable indices]({{< relref "query-language/schema.md#sortable-indices">}}), Dgraph sorts on the values and with the index in parallel and returns whichever result is computed first.

Sorted queries retrieve up to 1000 results by default. This can be changed with [first]({{< relref "query-language/pagination.md#first">}}).


Query Example: French director Jean-Pierre Jeunet's movies sorted by release date.

{{< runnable >}}
{
  me(func: allofterms(name@en, "Jean-Pierre Jeunet")) {
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

  genres(func: uid(genres), orderasc: name@en) {
    name@en
    ~genre (orderdesc: val(numGenres), first: 5) {
      name@en
      genres : val(numGenres)
    }
  }
}
{{< /runnable >}}

Sorting can also be performed by multiple predicates as shown below. If the values are equal for the
first predicate, then they are sorted by the second predicate and so on.

Query Example: Find all nodes which have type Person, sort them by their first_name and among those
that have the same first_name sort them by last_name in descending order.

```
{
  me(func: type("Person"), orderasc: first_name, orderdesc: last_name) {
    first_name
    last_name
  }
}
```
