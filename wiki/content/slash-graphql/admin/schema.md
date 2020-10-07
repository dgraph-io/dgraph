+++
title = "Fetching and Updating Your Schema"
weight = 3   
[menu.main]
    parent = "slash-graphql-admin"
+++

Your GraphQL schema can be fetched and updated using the `/admin` endpoint of your cluster. As an example, if your graphql endpoint is `https://frozen-mango-42.us-west-2.aws.cloud.dgraph.io/graphql`, then the admin endpoint for schema will be at `https://frozen-mango.us-west-2.aws.cloud.dgraph.io/admin`.

This endpoint works in a similar way to the [/admin](/graphql/admin) endpoint of Dgraph, with the additional constraint of [requiring authentication](/slash-graphql/admin/authentication).

### Fetching the Current Schema

It is possible to fetch your current schema using the `getGQLSchema` query on `/admin`. Below is a sample GraphQL query which will fetch this schema.

```graphql
{
  getGQLSchema {
    schema
  }
}
```

### Setting a New Schema

You can save a new schema using the `updateGQLSchema` mutation on `/admin`. Below is an example GraphQL body, with a variable called sch which must be passed in as a [variable](https://graphql.org/graphql-js/passing-arguments/)

```graphql
mutation($sch: String!) {
  updateGQLSchema(input: { set: { schema: $sch}})
  {
    gqlSchema {
      schema
      generatedSchema
    }
  }
}
```
