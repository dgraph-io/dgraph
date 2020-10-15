+++
date = "2017-03-20T22:25:17+11:00"
title = "Value Variables"
weight = 10
[menu.main]
    parent = "query-language"
+++


Syntax Examples:

* `varName as scalarPredicate`
* `varName as count(predicate)`
* `varName as avg(...)`
* `varName as math(...)`

Types : `int`, `float`, `String`, `dateTime`, `default`, `geo`, `bool`

Value variables store scalar values.  Value variables are a map from the UIDs of the enclosing block to the corresponding values.

It therefore only makes sense to use the values from a value variable in a context that matches the same UIDs - if used in a block matching different UIDs the value variable is undefined.

It is an error to define a value variable but not use it elsewhere in the query.

Value variables are used by extracting the values with `val(var-name)`, or by extracting the UIDs with `uid(var-name)`.

[Facet]({{< relref "query-language/facets.md">}}) values can be stored in value variables.

Query Example: The number of movie roles played by the actors of the 80's classic "The Princess Bride".  Query variable `pbActors` matches the UIDs of all actors from the movie.  Value variable `roles` is thus a map from actor UID to number of roles.  Value variable `roles` can be used in the `totalRoles` query block because that query block also matches the `pbActors` UIDs, so the actor to number of roles map is available.

{{< runnable >}}
{
  var(func:allofterms(name@en, "The Princess Bride")) {
    starring {
      pbActors as performance.actor {
        roles as count(actor.film)
      }
    }
  }
  totalRoles(func: uid(pbActors), orderasc: val(roles)) {
    name@en
    numRoles : val(roles)
  }
}
{{< /runnable >}}


Value variables can be used in place of UID variables by extracting the UID list from the map.

Query Example: The same query as the previous example, but using value variable `roles` for matching UIDs in the `totalRoles` query block.

{{< runnable >}}
{
  var(func:allofterms(name@en, "The Princess Bride")) {
    starring {
      performance.actor {
        roles as count(actor.film)
      }
    }
  }
  totalRoles(func: uid(roles), orderasc: val(roles)) {
    name@en
    numRoles : val(roles)
  }
}
{{< /runnable >}}


## Variable Propagation

Like query variables, value variables can be used in other query blocks and in blocks nested within the defining block.  When used in a block nested within the block that defines the variable, the value is computed as a sum of the variable for parent nodes along all paths to the point of use.  This is called variable propagation.

For example:
```
{
  q(func: uid(0x01)) {
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
    num_roles(func: eq(name@en, "Warwick Davis")) @cascade @normalize {

    paths as math(1)  # records number of paths to each character

    actor : name@en

    actor.film {
      performance.film @filter(allofterms(name@en, "Harry Potter")) {
        film_name : name@en
        characters : math(paths)  # how many paths (i.e. characters) reach this film
      }
    }
  }
}
{{< /runnable >}}


Query Example: Each actor who has been in a Peter Jackson movie and the fraction of Peter Jackson movies they have appeared in.
{{< runnable >}}
{
    movie_fraction(func:eq(name@en, "Peter Jackson")) @normalize {

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
