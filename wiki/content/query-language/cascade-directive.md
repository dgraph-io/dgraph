+++
date = "2017-03-20T22:25:17+11:00"
title = "Cascade Directive"
weight = 15
[menu.main]
    parent = "query-language"
+++

With the `@cascade` directive, nodes that don't have all predicates specified in the query are removed. This can be useful in cases where some filter was applied or if nodes might not have all listed predicates.


Query Example: Harry Potter movies, with each actor and characters played.  With `@cascade`, any character not played by an actor called Warwick is removed, as is any Harry Potter movie without any actors called Warwick.  Without `@cascade`, every character is returned, but only those played by actors called Warwick also have the actor name.
{{< runnable >}}
{
  HP(func: allofterms(name@en, "Harry Potter")) @cascade {
    name@en
    starring{
        performance.character {
          name@en
        }
        performance.actor @filter(allofterms(name@en, "Warwick")){
            name@en
         }
    }
  }
}
{{< /runnable >}}

You can apply `@cascade` on inner query blocks as well.
{{< runnable >}}
{
  HP(func: allofterms(name@en, "Harry Potter")) {
    name@en
    genre {
      name@en
    }
    starring @cascade {
        performance.character {
          name@en
        }
        performance.actor @filter(allofterms(name@en, "Warwick")){
            name@en
         }
    }
  }
}
{{< /runnable >}}