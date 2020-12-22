+++
title = "Admin"
weight = 12
[menu.main]
  name = "Admin"
  identifier = "graphql-admin"
  parent = "graphql"
+++

This article presents the Admin API and explains how to run a Dgraph database with GraphQL.

## Running Dgraph with GraphQL

The simplest way to start with Dgraph GraphQL is to run the all-in-one Docker image.

```
docker run -it -p 8080:8080 dgraph/standalone:master
```

That brings up GraphQL at `localhost:8080/graphql` and `localhost:8080/admin`, but is intended for quickstart and doesn't persist data.

## Advanced options

Once you've tried out Dgraph GraphQL, you'll need to move past the `dgraph/standalone` and run and deploy Dgraph instances.

Dgraph is a distributed graph database.  It can scale to huge data and shard that data across a cluster of Dgraph instances.  GraphQL is built into Dgraph in its Alpha nodes. To learn how to manage and deploy a Dgraph cluster, check our [deployment guide](https://dgraph.io/docs/deploy/).

GraphQL schema introspection is enabled by default, but can be disabled with the `--graphql_introspection=false` when starting the Dgraph alpha nodes.

## Dgraph's schema

Dgraph's GraphQL runs in Dgraph and presents a GraphQL schema where the queries and mutations are executed in the Dgraph cluster.  So the GraphQL schema is backed by Dgraph's schema.

{{% notice "warning" %}}
this means that if you have a Dgraph instance and change its GraphQL schema, the schema of the underlying Dgraph will also be changed!
{{% /notice %}}

## Endpoints

When you start Dgraph with GraphQL, two GraphQL endpoints are served.

### /graphql

At `/graphql` you'll find the GraphQL API for the types you've added.  That's what your app would access and is the GraphQL entry point to Dgraph.  If you need to know more about this, see the [quick start](https://dgraph.io/docs/graphql/quick-start/) and [schema docs](https://dgraph.io/docs/graphql/schema/).

### /admin

At `/admin` you'll find an admin API for administering your GraphQL instance.  The admin API is a GraphQL API that serves POST and GET as well as compressed data, much like the `/graphql` endpoint.

Here are the important types, queries, and mutations from the `admin` schema.

```graphql
	scalar DateTime

	"""
	Data about the GraphQL schema being served by Dgraph.
	"""
	type GQLSchema @dgraph(type: "dgraph.graphql") {
		id: ID!

		"""
		Input schema (GraphQL types) that was used in the latest schema update.
		"""
		schema: String!  @dgraph(pred: "dgraph.graphql.schema")

		"""
		The GraphQL schema that was generated from the 'schema' field.
		This is the schema that is being served by Dgraph at /graphql.
		"""
		generatedSchema: String!
	}

	type Cors @dgraph(type: "dgraph.cors"){
		acceptedOrigins: [String]
	}

	"""
	SchemaHistory contains the schema and the time when the schema has been created.
	"""
	type SchemaHistory @dgraph(type: "dgraph.graphql.history") {
		schema: String! @id @dgraph(pred: "dgraph.graphql.schema_history")
		created_at: DateTime! @dgraph(pred: "dgraph.graphql.schema_created_at")
	}

	"""
	A NodeState is the state of an individual Alpha or Zero node in the Dgraph cluster.
	"""
	type NodeState {

		"""
		Node type : either 'alpha' or 'zero'.
		"""
		instance: String

		"""
		Address of the node.
		"""
		address: String

		"""
		Node health status : either 'healthy' or 'unhealthy'.
		"""
		status: String

		"""
		The group this node belongs to in the Dgraph cluster.
		See : https://dgraph.io/docs/deploy/#cluster-setup.
		"""
		group: String

		"""
		Version of the Dgraph binary.
		"""
		version: String

		"""
		Time in nanoseconds since the node started.
		"""
		uptime: Int

		"""
		Time in Unix epoch time that the node was last contacted by another Zero or Alpha node.
		"""
		lastEcho: Int

		"""
		List of ongoing operations in the background.
		"""
		ongoing: [String]

		"""
		List of predicates for which indexes are built in the background.
		"""
		indexing: [String]

		"""
		List of Enterprise Features that are enabled.
		"""
		ee_features: [String]
	}

	type MembershipState {
		counter: Int
		groups: [ClusterGroup]
		zeros: [Member]
		maxLeaseId: Int
		maxTxnTs: Int
		maxRaftId: Int
		removed: [Member]
		cid: String
		license: License
	}

	type ClusterGroup {
		id: Int
		members: [Member]
		tablets: [Tablet]
		snapshotTs: Int
		checksum: Int
	}

	type Member {
		id: Int
		groupId: Int
		addr: String
		leader: Boolean
		amDead: Boolean
		lastUpdate: Int
		clusterInfoOnly: Boolean
		forceGroupId: Boolean
	}

	type Tablet {
		groupId: Int
		predicate: String
		force: Boolean
		space: Int
		remove: Boolean
		readOnly: Boolean
		moveTs: Int
	}

	type License {
		user: String
		maxNodes: Int
		expiryTs: Int
		enabled: Boolean
	}

	directive @dgraph(type: String, pred: String) on OBJECT | INTERFACE | FIELD_DEFINITION
	directive @secret(field: String!, pred: String) on OBJECT | INTERFACE


	type UpdateGQLSchemaPayload {
		gqlSchema: GQLSchema
	}

	input UpdateGQLSchemaInput {
		set: GQLSchemaPatch!
	}

	input GQLSchemaPatch {
		schema: String!
	}

	input ExportInput {
		format: String

		"""
		Destination for the backup: e.g. Minio or S3 bucket or /absolute/path
		"""
		destination: String

		"""
		Access key credential for the destination.
		"""
		accessKey: String

		"""
		Secret key credential for the destination.
		"""
		secretKey: String

		"""
		AWS session token, if required.
		"""
		sessionToken: String

		"""
		Set to true to allow backing up to S3 or Minio bucket that requires no credentials.
		"""
		anonymous: Boolean
	}

	type Response {
		code: String
		message: String
	}

	type ExportPayload {
		response: Response
		exportedFiles: [String]
	}

	type DrainingPayload {
		response: Response
	}

	type ShutdownPayload {
		response: Response
	}

	input ConfigInput {
		"""
		Estimated memory the caches can take. Actual usage by the process would be
		more than specified here. The caches will be updated according to the
        cache_percentage flag.
		"""
		cacheMb: Float

		"""
		True value of logRequest enables logging of all the requests coming to alphas.
		False value of logRequest disables above.
		"""
		logRequest: Boolean
	}

	type ConfigPayload {
		response: Response
	}

	type Config {
		cacheMb: Float
	}

	type Query {
		getGQLSchema: GQLSchema
		health: [NodeState]
		state: MembershipState
		config: Config
		getAllowedCORSOrigins: Cors
		querySchemaHistory(first: Int, offset: Int): [SchemaHistory]
	}

	type Mutation {

		"""
		Update the Dgraph cluster to serve the input schema.  This may change the GraphQL
		schema, the types and predicates in the Dgraph schema, and cause indexes to be recomputed.
		"""
		updateGQLSchema(input: UpdateGQLSchemaInput!) : UpdateGQLSchemaPayload

		"""
		Starts an export of all data in the cluster.  Export format should be 'rdf' (the default
		if no format is given), or 'json'.
		See : https://dgraph.io/docs/deploy/#export-database
		"""
		export(input: ExportInput!): ExportPayload

		"""
		Set (or unset) the cluster draining mode.  In draining mode no further requests are served.
		"""
		draining(enable: Boolean): DrainingPayload

		"""
		Shutdown this node.
		"""
		shutdown: ShutdownPayload

		"""
		Alter the node's config.
		"""
		config(input: ConfigInput!): ConfigPayload

		replaceAllowedCORSOrigins(origins: [String]): Cors

	}
```

You'll notice that the `/admin` schema is very much the same as the schemas generated by Dgraph GraphQL.

* The `health` query lets you know if everything is connected and if there's a schema currently being served at `/graphql`.
* The `state`  query returns the current state of the cluster and group membership information. For more information about `state` see [here](https://dgraph.io/docs/deploy/dgraph-zero/#more-about-state-endpoint).
* The `config` query returns the configuration options of the cluster set at the time of starting it.
* The `getGQLSchema` query gets the current GraphQL schema served at `/graphql`, or returns null if there's no such schema.
* The `getAllowedCORSOrigins` query returns your CORS policy.
* The `updateGQLSchema` mutation allows you to change the schema currently served at `/graphql`.

## Enterprise features

Enterprise Features like ACL, Backups and Restore are also available using the GraphQL API at `/admin` endpoint.

* [ACL](https://dgraph.io/docs/enterprise-features/access-control-lists/#using-graphql-admin-api)
* [Backups](https://dgraph.io/docs/enterprise-features/binary-backups/#create-a-backup)
* [Restore](https://dgraph.io/docs/enterprise-features/binary-backups/#restore-from-backup)

## First start

On first starting with a blank database:

* There's no schema served at `/graphql`.
* Querying the `/admin` endpoint for `getGQLSchema` returns `"getGQLSchema": null`.
* Querying the `/admin` endpoint for `health` lets you know that no schema has been added.

## Validating a schema

You can validate a GraphQL schema before adding it to your database by sending
your schema definition in an HTTP POST request to the to the
`/admin/schema/validate` endpoint, as shown in the following example:

Request header:

```ssh
path: /admin/schema/validate
method: POST
```

Request body:

```graphql
type Person {
	name: String
}
```

This endpoint returns a JSON response that indicates if the schema is valid or
not, and provides an error if isn't valid. In this case, the schema is valid,
so the JSON response includes the following message: `Schema is valid`.

## Modifying a schema

There are two ways you can modify a GraphQL schema:
- Using `/admin/schema`
- Using the `updateGQLSchema` mutation on `/admin`

### Using `/admin/schema`

The `/admin/schema` endpoint provides a simplified method to add and update schemas.

To create a schema you only need to call the `/admin/schema` endpoint with the required schema definition. For example:

```graphql
type Person {
	name: String
}
```

If you have the schema definition stored in a `schema.graphql` file, you can use `curl` like this:
```
curl -X POST localhost:8080/admin/schema --data-binary '@schema.graphql'
```

On successful execution, the `/admin/schema` endpoint will give you a JSON response with a success code.

### Using `updateGQLSchema` to add or modify a schema

Another option to add or modify a GraphQL schema is the `updateGQLSchema` mutation.

For example, to create a schema using `updateGQLSchema`, run this mutation on the `/admin` endpoint:

```graphql
mutation {
  updateGQLSchema(
    input: { set: { schema: "type Person { name: String }"}})
  {
    gqlSchema {
      schema
      generatedSchema
    }
  }
}
```

## Using `querySchemaHistory` to see schema history

You can query the history of your schema using `querySchemaHistory` on the
`/admin` endpoint. This allows you to debug any issues that arise as you iterate
your schema. You can specify how many entries to return, and an offset to skip
the first few entries in the query result.

Because a query using `querySchemaHistory` returns the complete schema
for each version, you can use the JSON returned by such a query to manually roll
back to an earlier schema version. To roll back, copy the desired
schema version from query results, and then send it to `updateGQLSchema`.

For example, to see the first 10 entries in your schema history, run the
following query on the `/admin` endpoint:

```graphql
query {
          querySchemaHistory ( first : 10 ){
              schema
              created_at
           }
}
```
You could also skip the first entry when querying your schema history by setting
an offset, as in the following example:

```graphql
query {
          querySchemaHistory ( first : 10, offset : 1 ){
              schema
              created_at
           }
}
```


## Initial schema

Regardless of the method used to upload the GraphQL schema, on a black database, adding this schema

```graphql
type Person {
	name: String
}
```

would cause the following:

* The `/graphql` endpoint would refresh and serve the GraphQL schema generated from type `type Person { name: String }`: that's Dgraph type `Person` and predicate `Person.name: string .` (see [this article](https://dgraph.io/docs/graphql/dgraph) on how to customize the generated schema)
* The schema of the underlying Dgraph instance would be altered to allow for the new `Person` type and `name` predicate.
* The `/admin` endpoint for `health` would return that a schema is being served.
* The mutation would return `"schema": "type Person { name: String }"` and the generated GraphQL schema for `generatedSchema` (this is the schema served at `/graphql`).
* Querying the `/admin` endpoint for `getGQLSchema` would return the new schema.

## Migrating a schema

Given an instance serving the GraphQL schema from the previous section, updating the schema to the following

```graphql
type Person {
    name: String @search(by: [regexp])
    dob: DateTime
}
```

would change the GraphQL definition of `Person` and result in the following:

* The `/graphql` endpoint would refresh and serve the GraphQL schema generated from the new type.
* The schema of the underlying Dgraph instance would be altered to allow for `dob` (predicate `Person.dob: datetime .` is added, and `Person.name` becomes `Person.name: string @index(regexp).`) and indexes are rebuilt to allow the regexp search.
* The `health` is unchanged.
* Querying the `/admin` endpoint for `getGQLSchema` would return the updated schema.

## Removing indexes from a schema

Adding a schema through GraphQL doesn't remove existing data (it only removes indexes).

For example, starting from the schema in the previous section and modifying it with the initial schema

```graphql
type Person {
	name: String
}
```

would have the following effects:

* The `/graphql` endpoint would refresh to serve the schema built from this type.
* Thus, field `dob` would no longer be accessible, and there'd be no search available on `name`.
* The search index on `name` in Dgraph would be removed.
* The predicate `dob` in Dgraph would be left untouched (the predicate remains and no data is deleted).
