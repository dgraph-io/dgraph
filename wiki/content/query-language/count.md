+++
date = "2017-03-20T22:25:17+11:00"
title = "Count"
weight = 6
[menu.main]
    parent = "query-language"
+++

Syntax Examples:

* `count(predicate)`
* `count(uid)`

The form `count(predicate)` counts how many `predicate` edges lead out of a node.

The form `count(uid)` counts the number of UIDs matched in the enclosing block.

Query Example: The number of films acted in by each actor with `Orlando` in their name.

{{< runnable >}}
{
  me(func: allofterms(name@en, "Orlando")) @filter(has(actor.film)) {
    name@en
    count(actor.film)
  }
}
{{< /runnable >}}

Count can be used at root and [aliased]({{< relref "query-language/alias.md" >}}).

Query Example: Count of directors who have directed more than five films.  When used at the query root, the [count index]({{< relref "query-language/schema.md#count-index" >}}) is required.

{{< runnable >}}
{
  directors(func: gt(count(director.film), 5)) {
    totalDirectors : count(uid)
  }
}
{{< /runnable >}}


Count can be assigned to a [value variable]({{< relref "query-language/value-variables.md">}}).

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

  edmw(func: uid(actors), orderdesc: val(totalRoles)) {
    name@en
    name@zh
    totalRoles : val(totalRoles)
  }
}
{{< /runnable >}}
