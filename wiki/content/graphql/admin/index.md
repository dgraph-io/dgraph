+++
title = "Admin"
[menu.main]
  url = "/graphql/admin/"
  name = "Admin"
  identifier = "graphql-admin"
  parent = "graphql"
  weight = 12
+++

The admin API and how to run Dgraph with GraphQL.

## Running

The simplest way to start with Dgraph GraphQL is to run the all-in-one Docker image.

```
docker run -it -p 8080:8080 dgraph/standalone:master
```

That brings up GraphQL at `localhost:8080/graphql` and `localhost:8080/admin`, but is intended for quickstart and doesn't persist data.

## Advanced Options

Once you've tried out Dgraph GraphQL, you'll need to move past the `dgraph/standalone` and run and deploy Dgraph instances.

Dgraph is a distributed graph database.  It can scale to huge data and shard that data across a cluster of Dgraph instances.  GraphQL is built into Dgraph in its Alpha nodes. To learn how to manage and deploy a Dgraph cluster to build an App check Dgraph's [Dgraph docs](https://docs.dgraph.io/), and, in particular, the [deployment guide](https://docs.dgraph.io/deploy/).

GraphQL schema introspection is enabled by default, but can be disabled with the `--graphql_introspection=false` when starting the Dgraph alpha nodes.

## Dgraph's schema

Dgraph's GraphQL runs in Dgraph and presents a GraphQL schema where the queries and mutations are executed in the Dgraph cluster.  So the GraphQL schema is backed by Dgraph's schema.

**Warning: this means if you have a Dgraph instance and change its GraphQL schema, the schema of the underlying Dgraph will also be changed!**

## /admin

When you start Dgraph with GraphQL, two GraphQL endpoints are served.

At `/graphql` you'll find the GraphQL API for the types you've added.  That's what your app would access and is the GraphQL entry point to Dgraph.  If you need to know more about this, see the quick start, example and schema docs.

At `/admin` you'll find an admin API for administering your GraphQL instance.  The admin API is a GraphQL API that serves POST and GET as well as compressed data, much like the `/graphql` endpoint.

Here are the important types, queries, and mutations from the admin schema.

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
		Estimated memory the LRU cache can take. Actual usage by the process would be
		more than specified here. (default -1 means no set limit)
		"""
		lruMb: Float

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
		lruMb: Float
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

You'll notice that the /admin schema is very much the same as the schemas generated by Dgraph GraphQL.

* The `health` query lets you know if everything is connected and if there's a schema currently being served at `/graphql`.
* The `state`  query returns the current state of the cluster and group membership information. For more information about `state` see [here](https://dgraph.io/docs/deploy/dgraph-zero/#more-about-state-endpoint). 
* The `config` query returns the configuration options of the cluster set at the time of starting it.
* The `getGQLSchema` query gets the current GraphQL schema served at `/graphql`, or returns null if there's no such schema.
* The `getAllowedCORSOrigins` query returns your CORS policy.
* The `updateGQLSchema` mutation allows you to change the schema currently served at `/graphql`.

## Enterprise Features

Enterprise Features like ACL, Backups and Restore are also available using the GraphQL API at `/admin` endpoint.

* [ACL](https://dgraph.io/docs/enterprise-features/access-control-lists/#using-graphql-admin-api).
* [Backups](https://dgraph.io/docs/enterprise-features/binary-backups/#create-a-backup).
* [Restore](https://dgraph.io/docs/enterprise-features/binary-backups/#restore-from-backup).

## First Start

On first starting with a blank database:

* There's no schema served at `/graphql`.
* Querying the `/admin` endpoint for `getGQLSchema` returns `"getGQLSchema": null`.
* Querying the `/admin` endpoint for `health` lets you know that no schema has been added.

## Adding Schema

Given a blank database, running the `/admin` mutation:

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

would cause the following.

* The `/graphql` endpoint would refresh and now serves the GraphQL schema generated from type `type Person { name: String }`: that's Dgraph type `Person` and predicate `Person.name: string .`; see [here](/graphql/dgraph) for how to customize the generated schema.
* The schema of the underlying Dgraph instance would be altered to allow for the new `Person` type and `name` predicate.
* The `/admin` endpoint for `health` would return that a schema is being served.
* The mutation returns `"schema": "type Person { name: String }"` and the generated GraphQL schema for `generatedSchema` (this is the schema served at `/graphql`).
* Querying the `/admin` endpoint for `getGQLSchema` would return the new schema.

## Migrating Schema

Given an instance serving the schema from the previous section, running an `updateGQLSchema` mutation with the following input

```graphql
type Person {
    name: String @search(by: [regexp])
    dob: DateTime
}
```

changes the GraphQL definition of `Person` and results in the following.

* The `/graphql` endpoint would refresh and now serves the GraphQL schema generated from the new type.
* The schema of the underlying Dgraph instance would be altered to allow for `dob` (predicate `Person.dob: datetime .` is added, and `Person.name` becomes `Person.name: string @index(regexp).`) and indexes are rebuilt to allow the regexp search.
* The `health` is unchanged.
* Querying the `/admin` endpoint for `getGQLSchema` now returns the updated schema.

## Removing from Schema

Adding a schema through GraphQL doesn't remove existing data (it would remove indexes).  For example, starting from the schema in the previous section and running `updateGQLSchema` with the initial `type Person { name: String }` would have the following effects.

* The `/graphql` endpoint would refresh to serve the schema built from this type.
* Thus field `dob` would no longer be accessible and there'd be no search available on `name`.
* The search index on `name` in Dgraph would be removed.
* The predicate `dob` in Dgraph is left untouched - the predicate remains and no data is deleted.
