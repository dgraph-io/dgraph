+++
date = "2017-03-20T22:25:17+11:00"
title = "IgnoreReflex directive"
weight = 18
[menu.main]
    parent = "query-language"
+++

The `@ignorereflex` directive forces the removal of child nodes that are reachable from themselves as a parent, through any path in the query result

Query Example: All the co-actors of Rutger Hauer.  Without `@ignorereflex`, the result would also include Rutger Hauer for every movie.

{{< runnable >}}
{
  coactors(func: eq(name@en, "Rutger Hauer")) @ignorereflex {
    actor.film {
      performance.film {
        starring {
          performance.actor {
            name@en
          }
        }
      }
    }
  }
}
{{< /runnable >}}