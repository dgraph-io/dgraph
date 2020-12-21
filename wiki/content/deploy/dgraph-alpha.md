+++
date = "2017-03-20T22:25:17+11:00"
title = "More about Dgraph Alpha"
weight = 7
[menu.main]
    parent = "deploy"
+++

On its HTTP port, a Dgraph Alpha exposes a number of admin endpoints.
{{% notice "warning" %}}
These HTTP endpoints are deprecated and will be removed in the next release. Please use the GraphQL endpoint at /admin.
{{% /notice %}}

* `/health?all` returns information about the health of all the servers in the cluster.
* `/admin/shutdown` initiates a proper [shutdown]({{< relref "deploy/dgraph-administration.md#shutting-down-database" >}}) of the Alpha.
* `/admin/export` initiates a data [export]({{< relref "deploy/dgraph-administration.md#exporting-database" >}}). The exported data will be
encrypted if the alpha instance was configured with an encryption key file.

By default the Alpha listens on `localhost` for admin actions (the loopback address only accessible from the same machine). The `--bindall=true` option binds to `0.0.0.0` and thus allows external connections.

{{% notice "tip" %}}Set max file descriptors to a high value like 10000 if you are going to load a lot of data.{{% /notice %}}

## Querying Health

You can query the `/admin` graphql endpoint with a query like the one below to get a JSON consisting of basic information about health of all the servers in the cluster.

```graphql
query {
  health {
    instance
    address
    version
    status
    lastEcho
    group
    uptime
    ongoing
    indexing
  }
}
```

Here’s an example of JSON returned from the above query:

```json
{
  "data": {
    "health": [
      {
        "instance": "zero",
        "address": "localhost:5080",
        "version": "v2.0.0-rc1",
        "status": "healthy",
        "lastEcho": 1582827418,
        "group": "0",
        "uptime": 1504
      },
      {
        "instance": "alpha",
        "address": "localhost:7080",
        "version": "v2.0.0-rc1",
        "status": "healthy",
        "lastEcho": 1582827418,
        "group": "1",
        "uptime": 1505,
        "ongoing": ["opIndexing"],
        "indexing": ["name", "age"]
      }
    ]
  }
}
```

- `instance`: Name of the instance. Either `alpha` or `zero`.
- `status`: Health status of the instance. Either `healthy` or `unhealthy`.
- `version`: Version of Dgraph running the Alpha or Zero server.
- `uptime`: Time in nanoseconds since the Alpha or Zero server is up and running.
- `address`: IP_ADDRESS:PORT of the instance.
- `group`: Group assigned based on the replication factor. Read more [here]({{< relref "/deploy/cluster-setup.md" >}}).
- `lastEcho`: Last time, in Unix epoch, when the instance was contacted by another Alpha or Zero server.
- `ongoing`: List of ongoing operations in the background.
- `indexing`: List of predicates for which indexes are built in the background. Read more [here]({{< relref "/query-language/schema.md#indexes-in-background" >}}).

The same information (except `ongoing` and `indexing`) is available from the `/health` and `/health?all` endpoints of Alpha server.
