+++
title = "DQL: Tips and Tricks"
[menu.main]
  identifier = "tips"
  parent = "dql"
  weight = 5
+++

## Get Sample Data

Use the `has` function to get some sample nodes.

{{< runnable >}}
{
  result(func: has(director.film), first: 10) {
    uid
    expand(_all_)
  }
}
{{< /runnable >}}


## Count number of connecting nodes

Use `expand(_all_)` to expand the nodes' edges, then assign them to a variable.
The variable can now be used to iterate over the unique neighboring nodes.
Then use `count(uid)` to count the number of nodes in a block.

{{< runnable >}}
{
  uids(func: has(director.film), first: 1) {
    uid
    expand(_all_) { u as uid }
  }

  result(func: uid(u)) {
    count(uid)
  }
}
{{< /runnable >}}

## Search on non-indexed predicates

Use the `has` function among the value variables to search on non-indexed predicates.

{{< runnable >}}
{
  var(func: has(festival.date_founded)) {
    p as festival.date_founded
  }
  query(func: eq(val(p), "1961-01-01T00:00:00Z")) {
      uid
      name@en
      name@ru
      name@pl
      festival.date_founded
      festival.focus { name@en }
      festival.individual_festivals { total : count(uid) }
  }
}
{{< /runnable >}}

## Sort edge by nested node values

Dgraph [sorting]({{< relref "query-language/sorting.md" >}}) is based on a single
level of the subgraph. To sort a level by the values of a deeper level, use
[query variables]({{< relref "query-language/query-variables.md" >}}) to bring
nested values up to the level of the edge to be sorted.

Example: Get all actors from a Steven Spielberg movie sorted alphabetically.
The actor's name is not accessed from a single traversal from the `starring` edge;
the name is accessible via `performance.actor`.

{{< runnable >}}
{
  spielbergMovies as var(func: allofterms(name@en, "steven spielberg")) {
    name@en
    director.film (orderasc: name@en, first: 1) {
      starring {
        performance.actor {
          ActorName as name@en
        }
        # Stars is a uid-to-value map mapping
        # starring edges to performance.actor names
        Stars as min(val(ActorName))
      }
    }
  }

  movies(func: uid(spielbergMovies)) @cascade {
    name@en
    director.film (orderasc: name@en, first: 1) {
      name@en
      starring (orderasc: val(Stars)) {
        performance.actor {
          name@en
        }
      }
    }
  }
}
{{< /runnable >}}

## Obtain unique results by using variables

To obtain unique results, assign the node's edge to a variable.
The variable can now be used to iterate over the unique nodes.

Example: Get all unique genres from all of the movies directed by Steven Spielberg.

{{< runnable >}}
{
  var(func: eq(name@en, "Steven Spielberg")) {
    director.film {
      genres as genre
    }
  }

  q(func: uid(genres)) {
    name@.
  }
}
{{< /runnable >}}

## Usage of checkpwd boolean

Store the result of `checkpwd` in a query variable and then match it against `1` (`checkpwd` is `true`) or `0` (`checkpwd` is `false`).

{{< runnable >}}
{  
  exampleData(func: has(email)) {
    uid
    email
    check as checkpwd(pass, "1bdfhJHb!fd")
  }
  userMatched(func: eq(val(check), 1)) {
    uid
    email
  }
  userIncorrect(func: eq(val(check), 0)) {
    uid
    email
  }
}
{{< /runnable >}}
