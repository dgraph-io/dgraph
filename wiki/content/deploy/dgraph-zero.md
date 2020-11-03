+++
date = "2017-03-20T22:25:17+11:00"
title = "More about Dgraph Zero"
weight = 8
[menu.main]
    parent = "deploy"
+++

Dgraph Zero controls the Dgraph cluster, and stores information about it. It
automatically moves data between different Dgraph Alpha instances based on the
size of the data served by each Alpha instance.

Before you can run `dgraph alpha`, you must run at least one `dgraph zero` node.
You can see the options available for `dgraph zero` by using the following command:

```bash
dgraph zero --help
```

The `--replicas` option controls the replication factor: the number
of replicas per data shard, including the original shard. For consensus, the
replication factor must be set to an odd number, and the following error will
occur if it is set to an even number (for example, `2`):

```nix
ERROR: Number of replicas must be odd for consensus. Found: 2
```

When a new Alpha joins the cluster, it is assigned to a group based on the replication factor. If the replication factor is set to `1`, then each Alpha
node will serve a different group. If the replication factor is set to `3` and
you then launch six Alpha nodes, the first three Alpha nodes will serve group 1
and next three nodes will serve group 2. Zero monitors the space occupied by predicates in each group and moves predicates between groups as-needed to
rebalance the cluster.

## Endpoints

Like Alpha, Zero also exposes HTTP on port 6080 (plus any ports specified by
`--port_offset`). You can query this port using a **GET** request to access the
following endpoints:

* `/state` returns information about the nodes that are part of the cluster. This
includes information about the size of predicates and which groups they belong
to.
* `/assign?what=uids&num=100` allocates a range of UIDs specified
by the `num` argument, and returns a JSON map containing the `startId` and
 `endId` that defines the range of UIDs (inclusive). This UID range can be
safely assigned externally to new nodes during data ingestion.
* `/assign?what=timestamps&num=100` requests timestamps from Zero. This is
useful to "fast forward" the state of the Zero node when starting from a
postings directory that already has commits higher than Zero's leased timestamp.
* `/removeNode?id=3&group=2` removes a dead Zero or Alpha node. When a replica
node goes offline and can't be recovered, you can remove it and add a new node to th
quorum. To remove dead Zero nodes, pass `group=0` and the id of the Zero node to
this endpoint.

{{% notice "note" %}}
Before using the API ensure that the node is down and ensure that it doesn't
come back up ever again. Do not use the same `idx` of a node that was removed
earlier.
{{% /notice %}}

* `/moveTablet?tablet=name&group=2` Moves a tablet to a group. Zero already
rebalances shards every 8 mins, but this endpoint can be used to force move a
tablet.

You can also use the following **POST** endpoint on HTTP port 6080:

* `/enterpriseLicense` applies an enterprise license to the
cluster by supplying it as part of the body.

### More About the /state Endpoint

The `/state` endpoint of Dgraph Zero returns a JSON document of the current
group membership info, which includes the following:

- Instances which are part of the cluster.
- Number of instances in Zero group and each Alpha groups.
- Current leader of each group.
- Predicates that belong to a group.
- Estimated size in bytes of each predicate.
- Enterprise license information.
- Max Leased transaction ID.
- Max Leased UID.
- CID (Cluster ID).

Here’s an example of JSON for a cluster with three Alpha nodes and three Zero
nodes returned from the `/state` endpoint:

```json
{
  "counter": "22",
  "groups": {
    "1": {
      "members": {
        "1": {
          "id": "1",
          "groupId": 1,
          "addr": "alpha2:7082",
          "leader": true,
          "amDead": false,
          "lastUpdate": "1603350485",
          "clusterInfoOnly": false,
          "forceGroupId": false
        },
        "2": {
          "id": "2",
          "groupId": 1,
          "addr": "alpha1:7080",
          "leader": false,
          "amDead": false,
          "lastUpdate": "0",
          "clusterInfoOnly": false,
          "forceGroupId": false
        },
        "3": {
          "id": "3",
          "groupId": 1,
          "addr": "alpha3:7083",
          "leader": false,
          "amDead": false,
          "lastUpdate": "0",
          "clusterInfoOnly": false,
          "forceGroupId": false
        }
      },
      "tablets": {
        "dgraph.cors": {
          "groupId": 1,
          "predicate": "dgraph.cors",
          "force": false,
          "space": "0",
          "remove": false,
          "readOnly": false,
          "moveTs": "0"
        },
        "dgraph.graphql.schema": {
          "groupId": 1,
          "predicate": "dgraph.graphql.schema",
          "force": false,
          "space": "0",
          "remove": false,
          "readOnly": false,
          "moveTs": "0"
        },
        "dgraph.graphql.schema_created_at": {
          "groupId": 1,
          "predicate": "dgraph.graphql.schema_created_at",
          "force": false,
          "space": "0",
          "remove": false,
          "readOnly": false,
          "moveTs": "0"
        },
        "dgraph.graphql.schema_history": {
          "groupId": 1,
          "predicate": "dgraph.graphql.schema_history",
          "force": false,
          "space": "0",
          "remove": false,
          "readOnly": false,
          "moveTs": "0"
        },
        "dgraph.graphql.xid": {
          "groupId": 1,
          "predicate": "dgraph.graphql.xid",
          "force": false,
          "space": "0",
          "remove": false,
          "readOnly": false,
          "moveTs": "0"
        },
        "dgraph.type": {
          "groupId": 1,
          "predicate": "dgraph.type",
          "force": false,
          "space": "0",
          "remove": false,
          "readOnly": false,
          "moveTs": "0"
        }
      },
      "snapshotTs": "22",
      "checksum": "18099480229465877561"
    }
  },
  "zeros": {
    "1": {
      "id": "1",
      "groupId": 0,
      "addr": "zero1:5080",
      "leader": true,
      "amDead": false,
      "lastUpdate": "0",
      "clusterInfoOnly": false,
      "forceGroupId": false
    },
    "2": {
      "id": "2",
      "groupId": 0,
      "addr": "zero2:5082",
      "leader": false,
      "amDead": false,
      "lastUpdate": "0",
      "clusterInfoOnly": false,
      "forceGroupId": false
    },
    "3": {
      "id": "3",
      "groupId": 0,
      "addr": "zero3:5083",
      "leader": false,
      "amDead": false,
      "lastUpdate": "0",
      "clusterInfoOnly": false,
      "forceGroupId": false
    }
  },
  "maxLeaseId": "10000",
  "maxTxnTs": "10000",
  "maxRaftId": "3",
  "removed": [],
  "cid": "2571d268-b574-41fa-ae5e-a6f8da175d6d",
  "license": {
    "user": "",
    "maxNodes": "18446744073709551615",
    "expiryTs": "1605942487",
    "enabled": true
  }
}
```

This JSON provides information that includes the following, with node members
shown with their node name and HTTP port number:

- Group 1 members:
    - alpha2:7082, id: 1, leader
    - alpha1:7080, id: 2
    - alpha3:7083, id: 3
- Group 0 members (Dgraph Zero nodes)
    - zero1:5080, id: 1, leader
    - zero2:5082, id: 2
    - zero3:5083, id: 3
- maxLeaseId
    - The current maximum lease of UIDs used for blank node UID assignment.
    - This increments in batches of 10,000 IDs. Once the maximum lease is
      reached, another 10,000 IDs are leased. In the event that the Zero
      leader is lost, the new leader starts a new lease from
      `maxLeaseId`+1. Any UIDs lost between these leases will never be used
      for blank-node UID assignment.
    - An admin can use the Zero endpoint HTTP GET `/assign?what=uids&num=1000` to
      reserve a range of UIDs (in this case, 1000) to use externally. Zero will
      **never** use these UIDs for blank node UID assignment, so the user can
      use the range to assign UIDs manually to their own data sets.
- maxTxnTs
    - The current maximum lease of transaction timestamps used to hand out
      start timestamps and commit timestamps. This increments in batches of
      10,000 IDs. After the max lease is reached, another 10,000 IDs are
      leased. If the Zero leader is lost, then the new leader starts a new
      lease from `maxTxnTs`+1 . Any lost transaction IDs between these
      leases will never be used.
    - An admin can use the Zero endpoint HTTP GET
      `/assign?what=timestamps&num=1000` to increase the current transaction
      timestamp (in this case, by 1000). This is mainly useful in
      special-case scenarios; for example, using an existing `-p directory` to
      create a fresh cluster to be able to query the latest data in the DB.
- maxRaftId
    - The number of Zeros available to serve as a leader node. Used by the
      [RAFT](/design-concepts/raft/) consensus algorithm.
- CID
    - This is a unique UUID representing the *cluster-ID* for this cluster. It
      is generated during the initial DB startup and is retained across
      restarts.
- Enterprise license
    - Enabled
    - maxNodes: unlimited
    - License expiration, shown in seconds since the Unix epoch.

{{% notice "note" %}}
The terms "tablet", "predicate", and "edge" are currently synonymous. In future,
Dgraph might improve data scalability to shard a predicate into separate tablets
that can be assigned to different groups.
{{% /notice %}}
