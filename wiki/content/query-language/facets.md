+++
date = "2017-03-20T22:25:17+11:00"
title = "Facets : Edge attributes"
weight = 22
[menu.main]
    parent = "query-language"
+++

Dgraph supports facets --- **key value pairs on edges** --- as an extension to RDF triples. That is, facets add properties to edges, rather than to nodes.
For example, a `friend` edge between two nodes may have a boolean property of `close` friendship.
Facets can also be used as `weights` for edges.

Though you may find yourself leaning towards facets many times, they should not be misused.  It wouldn't be correct modeling to give the `friend` edge a facet `date_of_birth`. That should be an edge for the friend.  However, a facet like `start_of_friendship` might be appropriate.  Facets are however not first class citizen in Dgraph like predicates.

Facet keys are strings and values can be `string`, `bool`, `int`, `float` and `dateTime`.
For `int` and `float`, only 32-bit signed integers and 64-bit floats are accepted.

The following mutation is used throughout this section on facets.  The mutation adds data for some peoples and, for example, records a `since` facet in `mobile` and `car` to record when Alice bought the car and started using the mobile number.

First we add some schema.
```sh
curl localhost:8080/alter -XPOST -d $'
    name: string @index(exact, term) .
    rated: [uid] @reverse @count .
' | python -m json.tool | less
```

```sh
curl -H "Content-Type: application/rdf" localhost:8080/mutate?commitNow=true -XPOST -d $'
{
  set {

    # -- Facets on scalar predicates
    _:alice <name> "Alice" .
    _:alice <dgraph.type> "Person" .
    _:alice <mobile> "040123456" (since=2006-01-02T15:04:05) .
    _:alice <car> "MA0123" (since=2006-02-02T13:01:09, first=true) .

    _:bob <name> "Bob" .
    _:bob <dgraph.type> "Person" .
    _:bob <car> "MA0134" (since=2006-02-02T13:01:09) .

    _:charlie <name> "Charlie" .
    _:charlie <dgraph.type> "Person" .
    _:dave <name> "Dave" .
    _:dave <dgraph.type> "Person" .


    # -- Facets on UID predicates
    _:alice <friend> _:bob (close=true, relative=false) .
    _:alice <friend> _:charlie (close=false, relative=true) .
    _:alice <friend> _:dave (close=true, relative=true) .


    # -- Facets for variable propagation
    _:movie1 <name> "Movie 1" .
    _:movie1 <dgraph.type> "Movie" .
    _:movie2 <name> "Movie 2" .
    _:movie2 <dgraph.type> "Movie" .
    _:movie3 <name> "Movie 3" .
    _:movie3 <dgraph.type> "Movie" .

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

## Facets on scalar predicates


Querying `name`, `mobile` and `car` of Alice gives the same result as without facets.

{{< runnable >}}
{
  data(func: eq(name, "Alice")) {
     name
     mobile
     car
  }
}
{{</ runnable >}}


The syntax `@facets(facet-name)` is used to query facet data. For Alice the `since` facet for `mobile` and `car` are queried as follows.

{{< runnable >}}
{
  data(func: eq(name, "Alice")) {
     name
     mobile @facets(since)
     car @facets(since)
  }
}
{{</ runnable >}}


Facets are returned at the same level as the corresponding edge and have keys like edge|facet.

All facets on an edge are queried with `@facets`.

{{< runnable >}}
{
  data(func: eq(name, "Alice")) {
     name
     mobile @facets
     car @facets
  }
}
{{</ runnable >}}

## Facets i18n

Facets keys and values can use language-specific characters directly when mutating. But facet keys need to be enclosed in angle brackets `<>` when querying. This is similar to predicates. See [Predicates i18n]({{< relref "query-language/schema.md#predicates-i18n" >}}) for more info.

{{% notice "note" %}}Dgraph supports [Internationalized Resource Identifiers](https://en.wikipedia.org/wiki/Internationalized_Resource_Identifier) (IRIs) for facet keys when querying.{{% /notice  %}}

Example:
```
{
  set {
    _:person1 <name> "Daniel" (वंश="स्पेनी", ancestry="Español") .
    _:person1 <dgraph.type> "Person" .
    _:person2 <name> "Raj" (वंश="हिंदी", ancestry="हिंदी") .
    _:person2 <dgraph.type> "Person" .
    _:person3 <name> "Zhang Wei" (वंश="चीनी", ancestry="中文") .
    _:person3 <dgraph.type> "Person" .
  }
}
```
Query, notice the `<>`'s:
```
{
  q(func: has(name)) {
    name @facets(<वंश>)
  }
}
```

## Alias with facets

Alias can be specified while requesting specific predicates. Syntax is similar to how would request
alias for other predicates. `orderasc` and `orderdesc` are not allowed as alias as they have special
meaning. Apart from that anything else can be set as alias.

Here we set `car_since`, `close_friend` alias for `since`, `close` facets respectively.
{{< runnable >}}
{
   data(func: eq(name, "Alice")) {
     name
     mobile
     car @facets(car_since: since)
     friend @facets(close_friend: close) {
       name
     }
   }
}
{{</ runnable >}}



## Facets on UID predicates

Facets on UID edges work similarly to facets on value edges.

For example, `friend` is an edge with facet `close`.
It was set to true for friendship between Alice and Bob
and false for friendship between Alice and Charlie.

A query for friends of Alice.

{{< runnable >}}
{
  data(func: eq(name, "Alice")) {
    name
    friend {
      name
    }
  }
}
{{</ runnable >}}

A query for friends and the facet `close` with `@facets(close)`.

{{< runnable >}}
{
   data(func: eq(name, "Alice")) {
     name
     friend @facets(close) {
       name
     }
   }
}
{{</ runnable >}}


For uid edges like `friend`, facets go to the corresponding child under the key edge|facet. In the above
example you can see that the `close` facet on the edge between Alice and Bob appears with the key `friend|close`
along with Bob's results.

{{< runnable >}}
{
  data(func: eq(name, "Alice")) {
    name
    friend @facets {
      name
      car @facets
    }
  }
}
{{</ runnable >}}

Bob has a `car` and it has a facet `since`, which, in the results, is part of the same object as Bob
under the key car|since.
Also, the `close` relationship between Bob and Alice is part of Bob's output object.
Charlie does not have `car` edge and thus only UID facets.

## Filtering on facets

Dgraph supports filtering edges based on facets.
Filtering works similarly to how it works on edges without facets and has the same available functions.


Find Alice's close friends
{{< runnable >}}
{
  data(func: eq(name, "Alice")) {
    friend @facets(eq(close, true)) {
      name
    }
  }
}
{{</ runnable >}}


To return facets as well as filter, add another `@facets(<facetname>)` to the query.

{{< runnable >}}
{
  data(func: eq(name, "Alice")) {
    friend @facets(eq(close, true)) @facets(relative) { # filter close friends and give relative status
      name
    }
  }
}
{{</ runnable >}}


Facet queries can be composed with `AND`, `OR` and `NOT`.

{{< runnable >}}
{
  data(func: eq(name, "Alice")) {
    friend @facets(eq(close, true) AND eq(relative, true)) @facets(relative) { # filter close friends in my relation
      name
    }
  }
}
{{</ runnable >}}


## Sorting using facets

Sorting is possible for a facet on a uid edge. Here we sort the movies rated by Alice, Bob and
Charlie by their `rating` which is a facet.

{{< runnable >}}
{
  me(func: anyofterms(name, "Alice Bob Charlie")) {
    name
    rated @facets(orderdesc: rating) {
      name
    }
  }
}
{{</ runnable >}}



## Assigning Facet values to a variable

Facets on UID edges can be stored in [value variables]({{< relref "query-language/value-variables.md" >}}).  The variable is a map from the edge target to the facet value.

Alice's friends reported by variables for `close` and `relative`.
{{< runnable >}}
{
  var(func: eq(name, "Alice")) {
    friend @facets(a as close, b as relative)
  }

  friend(func: uid(a)) {
    name
    val(a)
  }

  relative(func: uid(b)) {
    name
    val(b)
  }
}
{{</ runnable >}}


## Facets and Variable Propagation

Facet values of `int` and `float` can be assigned to variables and thus the [values propagate]({{< relref "query-language/value-variables.md#variable-propagation" >}}).


Alice, Bob and Charlie each rated every movie.  A value variable on facet `rating` maps movies to ratings.  A query that reaches a movie through multiple paths sums the ratings on each path.  The following sums Alice, Bob and Charlie's ratings for the three movies.

{{<runnable >}}
{
  var(func: anyofterms(name, "Alice Bob Charlie")) {
    num_raters as math(1)
    rated @facets(r as rating) {
      total_rating as math(r) # sum of the 3 ratings
      average_rating as math(total_rating / num_raters)
    }
  }
  data(func: uid(total_rating)) {
    name
    val(total_rating)
    val(average_rating)
  }

}
{{</ runnable >}}



## Facets and Aggregation

Facet values assigned to value variables can be aggregated.

{{< runnable >}}
{
  data(func: eq(name, "Alice")) {
    name
    rated @facets(r as rating) {
      name
    }
    avg(val(r))
  }
}
{{</ runnable >}}


Note though that `r` is a map from movies to the sum of ratings on edges in the query reaching the movie.  Hence, the following does not correctly calculate the average ratings for Alice and Bob individually --- it calculates 2 times the average of both Alice and Bob's ratings.

{{< runnable >}}

{
  data(func: anyofterms(name, "Alice Bob")) {
    name
    rated @facets(r as rating) {
      name
    }
    avg(val(r))
  }
}
{{</ runnable >}}

Calculating the average ratings of users requires a variable that maps users to the sum of their ratings.

{{< runnable >}}

{
  var(func: has(rated)) {
    num_rated as math(1)
    rated @facets(r as rating) {
      avg_rating as math(r / num_rated)
    }
  }

  data(func: uid(avg_rating)) {
    name
    val(avg_rating)
  }
}
{{</ runnable >}}
