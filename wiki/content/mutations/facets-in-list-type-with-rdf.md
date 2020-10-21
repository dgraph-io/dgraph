+++
date = "2017-03-20T22:25:17+11:00"
title = "Facets in List-type with RDF"
weight = 8
[menu.main]
    parent = "mutations"
+++

Schema:

```sh
<name>: string @index(exact).
<nickname>: [string] .
```

Creating a list with facets in RDF is straightforward.

```sh
{
  set {
    _:Julian <name> "Julian" .
    _:Julian <nickname> "Jay-Jay" (kind="first") .
    _:Julian <nickname> "Jules" (kind="official") .
    _:Julian <nickname> "JB" (kind="CS-GO") .
  }
}
```

```graphql
{
  q(func: eq(name,"Julian")){
    name
    nickname @facets
  }
}
```
Result:
```JSON
{
  "data": {
    "q": [
      {
        "name": "Julian",
        "nickname|kind": {
          "0": "first",
          "1": "official",
          "2": "CS-GO"
        },
        "nickname": [
          "Jay-Jay",
          "Jules",
          "JB"
        ]
      }
    ]
  }
}
```