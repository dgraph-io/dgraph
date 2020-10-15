+++
title = "Importing and Exporting data from Slash GraphQL"
weight = 4   
[menu.main]
    parent = "slash-graphql-admin"
+++

It is possible to export your data from one slash backend, and then import this data back into another Dgraph instance or Slash Backend.

## Exporting Data

It is possible to export your data via a JSON format. In order to do this, call the `export` mutation on `/admin/slash`. As an example, if your graphql endpoint is `https://frozen-mango-42.us-west-2.aws.cloud.dgraph.io/graphql`, then the admin endpoint for schema will be at `https://frozen-mango.us-west-2.aws.cloud.dgraph.io/admin/slash`.

Please note that this endpoint requires [Authentication](/slash-graphql/admin/authentication).

Below is a sample GraphQL body to export data to JSON.

```graphql
mutation {
  export {
    response { code message }
    signedUrls
  }
}
```

The `signedUrls` output field contains a list of URLs which can be downloaded. The URLs will expire after 48 hours.

Export will usually return 3 files:
* g01.gql_schema.gz - The GraphQL schema file. This file can be reimported via the [Schema APIs](/slash-graphql/admin/schema)
* g01.json.gz - the data from your instance, which can be imported via live loader
* g01.schema.gz - This file is the internal Dgraph schema. If you have set up your backend with a GraphQL schema, then you should be able to ignore this file.

## Importing data with Live Loader

It is possible to import data into a Slash GraphQL backend using [live loader](https://dgraph.io/docs/deploy/#live-loader). In order to import data, do the following steps

1. First import your schema into your Slash GraphQL backend, using either the [Schema API](/slash-graphql/admin/schema) or via [the Schema Page](https://slash.dgraph.io/_/schema).
2. Find the gRPC endpoint for your cluster, as described in the [advanced queries](/slash-graphql/advanced-queries) section. This will look like frozen-mango-42.grpc.us-west-1.aws.cloud.dgraph.io:443
3. Run the live loader as follows. Do note that running this via docker requires you to use an unreleased tag (either master or v20.07-slash)

```
docker run -it --rm -v /path/to/g01.json.gz:/tmp/g01.json.gz dgraph/dgraph:v20.07-slash \
  dgraph live --slash_grpc_endpoint=<grpc-endpoint>:443 -f /tmp/g01.json.gz -t <api-token>
```
