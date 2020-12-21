+++
date = "2017-03-20T22:25:17+11:00"
title = "Loading CSV Data"
weight = 6
[menu.main]
    parent = "migration"
+++

[Dgraph mutations]({{< relref "mutations/_index.md" >}}) are accepted in RDF
N-Quad and JSON formats. To load CSV-formatted data into Dgraph, first convert
the dataset into one of the accepted formats and then load the resulting dataset
into Dgraph. This section demonstrates converting CSV into JSON. 

{{% notice "tip" %}}
Once you have converted your `.csv` files to [RDF N-Quad/Triple](https://www.w3.org/TR/n-quads/) or JSON, 
you can use [Dgraph Live Loader]({{< relref "/deploy/fast-data-loading/live-loader.md" >}}) or 
[Dgraph Bulk Loader]({{< relref "/deploy/fast-data-loading/bulk-loader.md" >}}) to import your data.
{{% /notice %}}

There are many tools available to convert CSV to JSON. For example, you can use
[`d3-dsv`](https://github.com/d3/d3-dsv)'s `csv2json` tool as shown below:

```csv
Name,URL
Dgraph,https://github.com/dgraph-io/dgraph
Badger,https://github.com/dgraph-io/badger
```

```sh
$ csv2json names.csv --out names.json
$ cat names.json | jq '.'
[
  {
    "Name": "Dgraph",
    "URL": "https://github.com/dgraph-io/dgraph"
  },
  {
    "Name": "Badger",
    "URL": "https://github.com/dgraph-io/badger"
  }
]
```

This JSON can be loaded into Dgraph via the programmatic clients. This follows
the [JSON Mutation Format]({{< relref "mutations/json-mutation-format.md" >}}).
Note that each JSON object in the list above will be assigned a unique UID since
the `uid` field is omitted.

[The Ratel UI (and HTTP clients) expect JSON data to be stored within the `"set"`
key]({{< relref "mutations/json-mutation-format.md#json-syntax-using-raw-http-or-ratel-ui"
>}}). You can use `jq` to transform the JSON into the correct format:

```sh
$ cat names.json | jq '{ set: . }'
```
```json
{
  "set": [
    {
      "Name": "Dgraph",
      "URL": "https://github.com/dgraph-io/dgraph"
    },
    {
      "Name": "Badger",
      "URL": "https://github.com/dgraph-io/badger"
    }
  ]
}
```

Let's say you have CSV data in a file named connects.csv that's connecting nodes
together. Here, the `connects` field should `uid` type.

```csv
uid,connects
_:a,_:b
_:a,_:c
_:c,_:d
_:d,_:a
```

{{% notice "note" %}}
To reuse existing integer IDs from a CSV file as UIDs in Dgraph, use Dgraph Zero's [assign endpoint]({{< relref "deploy/dgraph-zero" >}}) before data loading to allocate a range of UIDs that can be safely assigned.
{{% /notice %}}

To get the correct JSON format, you can convert the CSV into JSON and use `jq`
to transform it in the correct format where the `connects` edge is a node uid:

```sh
$ csv2json connects.csv | jq '[ .[] | { uid: .uid, connects: { uid: .connects } } ]'
```

```json
[
  {
    "uid": "_:a",
    "connects": {
      "uid": "_:b"
    }
  },
  {
    "uid": "_:a",
    "connects": {
      "uid": "_:c"
    }
  },
  {
    "uid": "_:c",
    "connects": {
      "uid": "_:d"
    }
  },
  {
    "uid": "_:d",
    "connects": {
      "uid": "_:a"
    }
  }
]
```

You can modify the `jq` transformation to output the mutation format accepted by
Ratel UI and HTTP clients:

```sh
$ csv2json connects.csv | jq '{ set: [ .[] | {uid: .uid, connects: { uid: .connects } } ] }'
```
```json
{
  "set": [
    {
      "uid": "_:a",
      "connects": {
        "uid": "_:b"
      }
    },
    {
      "uid": "_:a",
      "connects": {
        "uid": "_:c"
      }
    },
    {
      "uid": "_:c",
      "connects": {
        "uid": "_:d"
      }
    },
    {
      "uid": "_:d",
      "connects": {
        "uid": "_:a"
      }
    }
  ]
}
```
