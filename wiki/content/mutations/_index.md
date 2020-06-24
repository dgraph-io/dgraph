+++
date = "2017-03-20T19:35:35+11:00"
title = "Mutations"
[menu.main]
  url = "/mutations/"
  identifier = "mutations"
  weight = 6
+++

Adding or removing data in Dgraph is called a mutation.

A mutation that adds triples is done with the `set` keyword.
```
{
  set {
    # triples in here
  }
}
```