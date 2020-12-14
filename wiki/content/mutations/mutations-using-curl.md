+++
date = "2017-03-20T22:25:17+11:00"
title = "Mutations using cURL"
weight = 9
[menu.main]
    parent = "mutations"
+++

Mutations can be done over HTTP by making a `POST` request to an Alpha's `/mutate` endpoint. On the command line this can be done with curl. To commit the mutation, pass the parameter `commitNow=true` in the URL.

To run a `set` mutation:

```sh
curl -H "Content-Type: application/rdf" -X POST localhost:8080/mutate?commitNow=true -d $'
{
  set {
    _:alice <name> "Alice" .
    _:alice <dgraph.type> "Person" .
  }
}'
```

To run a `delete` mutation:

```sh
curl -H "Content-Type: application/rdf" -X POST localhost:8080/mutate?commitNow=true -d $'
{
  delete {
    # Example: The UID of Alice is 0x56f33
    <0x56f33> <name> * .
  }
}'
```

To run an RDF mutation stored in a file, use curl's `--data-binary` option so that, unlike the `-d` option, the data is not URL encoded.

```sh
curl -H "Content-Type: application/rdf" -X POST localhost:8080/mutate?commitNow=true --data-binary @mutation.txt
```