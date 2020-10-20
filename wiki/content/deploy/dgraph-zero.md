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
You can see the options available for `dgraph zero` using the following command:

`dgraph zero --help`

* `--replicas` is the option that controls the replication factor. (i.e. number of replicas per data shard, including the original shard)
* When a new Alpha joins the cluster, it is assigned a group based on the replication factor. If the replication factor is 1 then each Alpha node will serve different group. If replication factor is 2 and you launch 4 Alphas, then first two Alphas would serve group 1 and next two machines would serve group 2.
* Zero also monitors the space occupied by predicates in each group and moves them around to rebalance the cluster.

Like Alpha, Zero also exposes HTTP on 6080 (+ any `--port_offset`). You can query (**GET** request) it
to see useful information, like the following:

* `/state` Information about the nodes that are part of the cluster. Also contains information about
size of predicates and groups they belong to.
* `/assign?what=uids&num=100` This would allocate `num` uids and return a JSON map
containing `startId` and `endId`, both inclusive. This id range can be safely assigned
externally to new nodes during data ingestion.
* `/assign?what=timestamps&num=100` This would request timestamps from Zero.
This is useful to fast forward Zero state when starting from a postings
directory, which already has commits higher than Zero's leased timestamp.
* `/removeNode?id=3&group=2` If a replica goes down and can't be recovered, you
can remove it and add a new node to the quorum. This endpoint can be used to
remove a dead Zero or Dgraph Alpha node. To remove dead Zero nodes, pass
`group=0` and the id of the Zero node.

{{% notice "note" %}}
Before using the API ensure that the node is down and ensure that it doesn't come back up ever again.

You should not use the same `idx` of a node that was removed earlier.
{{% /notice %}}

* `/moveTablet?tablet=name&group=2` This endpoint can be used to move a tablet to a group. Zero
already does shard rebalancing every 8 mins, this endpoint can be used to force move a tablet.


These are the **POST** endpoints available:

* `/enterpriseLicense` Use endpoint to apply an enterprise license to the cluster by supplying it
as part of the body.

## More about /state endpoint

The `/state` endpoint of Dgraph Zero returns a JSON document of the current group membership info:

- Instances which are part of the cluster.
- Number of instances in Zero group and each Alpha groups.
- Current leader of each group.
- Predicates that belong to a group.
- Estimated size in bytes of each predicate.
- Enterprise license information.
- Max Leased transaction ID.
- Max Leased UID.
- CID (Cluster ID).

Here’s an example of JSON returned from the `/state` endpoint:

```json
{
  "zeros": {
    "1": {
      "id": "1",
      "groupId": 0,
      "addr": "localhost:5080",
      "leader": true,
      "amDead": false,
      "lastUpdate": "0",
      "clusterInfoOnly": false,
      "forceGroupId": false
    },
    "2": {
      "id": "2",
      "groupId": 0,
      "addr": "localhost:5081",
      "leader": false,
      "amDead": false,
      "lastUpdate": "0",
      "clusterInfoOnly": false,
      "forceGroupId": false
    }
  },
  "maxLeaseId": "0",
  "maxTxnTs": "10000",
  "maxRaftId": "2",
  "removed": [],
  "cid": "cdcb1edb-8c81-4557-af99-ebed2b383e3c",
  "license": {
    "user": "",
    "maxNodes": "18446744073709551615",
    "expiryTs": "1597232699",
    "enabled": true
  }
}
```

The JSON document above provides the following information:

- Group 1
  - members
    - zero1:5080, id: 1, leader
    - zero2:5081, id: 2
- Enterprise license
    - Enabled
    - maxNodes: unlimited
    - License expiration, shown in seconds since the Unix epoch.
- Other data:
    - maxTxnTs
        - The current maximum lease of transaction timestamps used to hand out
          start timestamps and commit timestamps. This increments in batches of
          10,000 IDs. After the max lease is reached, another 10,000 IDs are
          leased. If the Zero leader is lost, then the new leader starts a new
          lease from `maxTxnTs`+1 . Any lost transaction IDs between these
          leases will never be used.
        - An admin can use the Zero endpoint HTTP GET `/assign?what=timestamps&num=1000` to
          increase the current transaction timestamp (in this case, by 1000).
          This is mainly useful in special-case scenarios; for example, using an
          existing `-p directory` to create a fresh cluster to be able to query the
          latest data in the DB.
    - maxRaftId
        - The number of Zeros available to serve as a leader node. Used by the
          [RAFT](/design-concepts/raft/) consensus algorithm.
    - maxLeaseId
        - The current maximum lease of UIDs used for blank node UID assignment.
        - This increments in batches of 10,000 IDs. Once the maximum lease is
          reached, another 10,000 IDs are leased. In the event that the Zero
          leader is lost, the new leader starts a new lease from
          `maxLeaseId`+1. Any UIDs lost between these leases will never be used
          for blank-node UID assignment.
        - An admin can use the Zero endpoint HTTP GET `/assign?what=uids&num=1000` to
          reserve a range of UIDs (in this case, 1000) to use externally. Zero will NEVER
          use these UIDs for blank node UID assignment, so the user can use the range
          to assign UIDs manually to their own data sets.
    - CID
        - This is a unique UUID representing the *cluster-ID* for this cluster. It is generated
          during the initial DB startup and is retained across restarts.


{{% notice "note" %}}
"tablet", "predicate", and "edge" are synonymous terms today. The future plan to
improve data scalability is to shard a predicate into separate tablets that could
be assigned to different groups.
{{% /notice %}}
