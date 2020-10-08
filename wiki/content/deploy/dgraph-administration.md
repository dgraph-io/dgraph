+++
date = "2017-03-20T22:25:17+11:00"
title = "Dgraph Administration"
weight = 18
[menu.main]
    parent = "deploy"
+++

Each Dgraph Alpha exposes administrative operations over HTTP to export data and to perform a clean shutdown.

## Whitelisting Admin Operations

By default, admin operations can only be initiated from the machine on which the Dgraph Alpha runs.
You can use the `--whitelist` option to specify whitelisted IP addresses and ranges for hosts from which admin operations can be initiated.

```sh
dgraph alpha --whitelist 172.17.0.0:172.20.0.0,192.168.1.1 ...
```
This would allow admin operations from hosts with IP between `172.17.0.0` and `172.20.0.0` along with
the server which has IP address as `192.168.1.1`.

## Restricting Mutation Operations

By default, you can perform mutation operations for any predicate.
If the predicate in mutation doesn't exist in the schema,
the predicate gets added to the schema with an appropriate
[Dgraph Type]({{< relref "query-language/schema.md" >}}).

You can use `--mutations disallow` to disable all mutations,
which is set to `allow` by default.

```sh
dgraph alpha --mutations disallow
```

Enforce a strict schema by setting `--mutations strict`.
This mode allows mutations only on predicates already in the schema.
Before performing a mutation on a predicate that doesn't exist in the schema,
you need to perform an alter operation with that predicate and its schema type.

```sh
dgraph alpha --mutations strict
```

## Securing Alter Operations

Clients can use alter operations to apply schema updates and drop particular or all predicates from the database.
By default, all clients are allowed to perform alter operations.
You can configure Dgraph to only allow alter operations when the client provides a specific token.
This can be used to prevent clients from making unintended or accidental schema updates or predicate drops.

You can specify the auth token with the `--auth_token` option for each Dgraph Alpha in the cluster.
Clients must include the same auth token to make alter requests.

```sh
$ dgraph alpha --auth_token=<authtokenstring>
```

```sh
$ curl -s localhost:8080/alter -d '{ "drop_all": true }'
# Permission denied. No token provided.
```

```sh
$ curl -s -H 'X-Dgraph-AuthToken: <wrongsecret>' localhost:8180/alter -d '{ "drop_all": true }'
# Permission denied. Incorrect token.
```

```sh
$ curl -H 'X-Dgraph-AuthToken: <authtokenstring>' localhost:8180/alter -d '{ "drop_all": true }'
# Success. Token matches.
```

{{% notice "note" %}}
To fully secure alter operations in the cluster, the auth token must be set for every Alpha.
{{% /notice %}}


## Exporting Database

An export of all nodes is started by locally executing the following GraphQL mutation on /admin endpoint using any compatible client like Insomnia, GraphQL Playground or GraphiQL.

```graphql
mutation {
  export(input: {format: "rdf"}) {
    response {
      message
      code
    }
  }
}
```
{{% notice "warning" %}}By default, this won't work if called from outside the server where the Dgraph Alpha is running.
You can specify a list or range of whitelisted IP addresses from which export or other admin operations
can be initiated using the `--whitelist` flag on `dgraph alpha`.
{{% /notice %}}

This triggers an export for all Alpha groups of the cluster. The data is exported from the following Dgraph instances:

1. For the Alpha instance that receives the GET request, the group's export data is stored with this Alpha.
2. For every other group, its group's export data is stored with the Alpha leader of that group.

It is up to the user to retrieve the right export files from the Alphas in the
cluster. Dgraph does not copy all files to the Alpha that initiated the export.
The user must also ensure that there is sufficient space on disk to store the
export.

Each Alpha leader for a group writes output as a gzipped file to the export
directory specified via the `--export` flag (defaults to a directory called `"export"`). If any of the groups fail, the
entire export process is considered failed and an error is returned.

The data is exported in RDF format by default. A different output format may be specified with the
`format` field. For example:

```graphql
mutation {
  export(input: {format: "json"}) {
    response {
      message
      code
    }
  }
}
```

Currently, "rdf" and "json" are the only formats supported.

### Encrypting Exports

Export is available wherever an Alpha is running. To encrypt an export, the Alpha must be configured with the `encryption-key-file`.

{{% notice "note" %}}
The `encryption-key-file` was used for `encryption-at-rest` and will now also be used for encrypted backups and exports.
{{% /notice %}}

## Shutting Down Database

A clean exit of a single Dgraph node is initiated by running the following GraphQL mutation on /admin endpoint.
{{% notice "warning" %}}This won't work if called from outside the server where Dgraph is running.
You can specify a list or range of whitelisted IP addresses from which shutdown or other admin operations
can be initiated using the `--whitelist` flag on `dgraph alpha`.
{{% /notice %}}

```graphql
mutation {
  shutdown {
    response {
      message
      code
    }
  }
}
```

This stops the Alpha on which the command is executed and not the entire cluster.

## Deleting database

Individual triples, patterns of triples and predicates can be deleted as described in the [DQL docs]({{< relref "mutations/delete.md" >}}).

To drop all data, you could send a `DropAll` request via `/alter` endpoint.

Alternatively, you could:

* [Shutdown Dgraph]({{< relref "#shutting-down-database" >}}) and wait for all writes to complete,
* Delete (maybe do an export first) the `p` and `w` directories, then
* Restart Dgraph.

## Upgrading Database

Doing periodic exports is always a good idea. This is particularly useful if you wish to upgrade Dgraph or reconfigure the sharding of a cluster. The following are the right steps to safely export and restart.

1. Start an [export]({{< relref "#exporting-database">}})
2. Ensure it is successful
3. [Shutdown Dgraph]({{< relref "#shutting-down-database" >}}) and wait for all writes to complete
4. Start a new Dgraph cluster using new data directories (this can be done by passing empty directories to the options `-p` and `-w` for Alphas and `-w` for Zeros)
5. Reload the data via [bulk loader]({{< relref "deploy/fast-data-loading.md#bulk-loader" >}})
6. Verify the correctness of the new Dgraph cluster. If all looks good, you can delete the old directories (export serves as an insurance)

These steps are necessary because Dgraph's underlying data format could have changed, and reloading the export avoids encoding incompatibilities.

Blue-green deployment is a common approach to minimize downtime during the upgrade process.
This approach involves switching your application to read-only mode. To make sure that no mutations are executed during the maintenance window you can
do a rolling restart of all your Alpha using the option `--mutations disallow` when you restart the Alphas. This will ensure the cluster is in read-only mode.

At this point your application can still read from the old cluster and you can perform the steps 4. and 5. described above.
When the new cluster (that uses the upgraded version of Dgraph) is up and running, you can point your application to it, and shutdown the old cluster.

### Upgrading from v1.2.2 to v20.03.0 for Enterprise Customers
<!-- TODO: Redirect(s) -->
1. Use [binary]({{< relref "enterprise-features/binary-backups.md">}}) backup to export data from old cluster
2. Ensure it is successful
3. [Shutdown Dgraph]({{< relref "#shutting-down-database" >}}) and wait for all writes to complete
4. Upgrade `dgraph` binary to `v20.03.0`
5. [Restore]({{< relref "enterprise-features/binary-backups.md#restore-from-backup">}}) from the backups using upgraded `dgraph` binary
6. Start a new Dgraph cluster using the restored data directories
7. Upgrade ACL data using the following command:

```
dgraph upgrade --acl -a localhost:9080 -u groot -p password
```

### Upgrading from v20.03.0 to v20.07.0 for Enterprise Customers
1. Use [binary]({{< relref "enterprise-features/binary-backups.md">}}) backup to export data from old cluster
2. Ensure it is successful
3. [Shutdown Dgraph]({{< relref "#shutting-down-database" >}}) and wait for all writes to complete
4. Upgrade `dgraph` binary to `v20.07.0`
5. [Restore]({{< relref "enterprise-features/binary-backups.md#restore-from-backup">}}) from the backups using upgraded `dgraph` binary
6. Start a new Dgraph cluster using the restored data directories
7. Upgrade ACL data using the following command:
    ```
    dgraph upgrade --acl -a localhost:9080 -u groot -p password -f v20.03.0 -t v20.07.0
    ```
    This is required because previously the type-names `User`, `Group` and `Rule` were used by ACL.
    They have now been renamed as `dgraph.type.User`, `dgraph.type.Group` and `dgraph.type.Rule`, to
    keep them in dgraph's internal namespace. This upgrade just changes the type-names for the ACL
    nodes to the new type-names.
    
    You can use `--dry-run` option in `dgraph upgrade` command to see a dry run of what the upgrade
    command will do.
8. If you have types or predicates in your schema whose names start with `dgraph.`, then
you would need to manually alter schema to change their names to something else which isn't
prefixed with `dgraph.`, and also do mutations to change the value of `dgraph.type` edge to the
new type name and copy data from old predicate name to new predicate name for all the nodes which
are affected. Then, you can drop the old types and predicates from DB.

{{% notice "note" %}}
If you are upgrading from v1.0, please make sure you follow the schema migration steps described in [this section]({{< relref "howto/migrate-dgraph-1-1.md" >}}).
{{% /notice %}}

## Post Installation

Now that Dgraph is up and running, to understand how to add and query data to Dgraph, follow [Query Language Spec](/query-language). Also, have a look at [Frequently asked questions](/faq).
