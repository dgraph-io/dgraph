+++
date = "2017-03-20T22:25:17+11:00"
title = "More about Dgraph Zero"
weight = 8
[menu.main]
    parent = "deploy"
+++

Dgraph Zero controls the Dgraph cluster. It automatically moves data between
different Dgraph Alpha instances based on the size of the data served by each Alpha instance.

It is mandatory to run at least one `dgraph zero` node before running any `dgraph alpha`.
Options present for `dgraph zero` can be seen by running `dgraph zero --help`.

* Zero stores information about the cluster.
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

Here’s an example of JSON returned from `/state` endpoint for a 6-node Dgraph cluster with three replicas:

```json
{
  "counter": "15",
  "groups": {
    "1": {
      "members": {
        "1": {
          "id": "1",
          "groupId": 1,
          "addr": "alpha1:7080",
          "leader": true,
          "lastUpdate": "1576112366"
        },
        "2": {
          "id": "2",
          "groupId": 1,
          "addr": "alpha2:7080"
        },
        "3": {
          "id": "3",
          "groupId": 1,
          "addr": "alpha3:7080"
        }
      },
      "tablets": {
        "counter.val": {
          "groupId": 1,
          "predicate": "counter.val"
        },
        "dgraph.type": {
          "groupId": 1,
          "predicate": "dgraph.type"
        }
      },
      "checksum": "1021598189643258447"
    }
  },
  "zeros": {
    "1": {
      "id": "1",
      "addr": "zero1:5080",
      "leader": true
    },
    "2": {
      "id": "2",
      "addr": "zero2:5080"
    },
    "3": {
      "id": "3",
      "addr": "zero3:5080"
    }
  },
  "maxLeaseId": "10000",
  "maxTxnTs": "10000",
  "cid": "3602537a-ee49-43cb-9792-c766eea683dc",
  "license": {
    "maxNodes": "18446744073709551615",
    "expiryTs": "1578704367",
    "enabled": true
  }
}
```

Here’s the information the above JSON document provides:

- Group 0
  - members
    - zero1:5080, id: 1, leader
    - zero2:5080, id: 2
    - zero3:5080, id: 3
- Group 1
    - members
        - alpha1:7080, id: 1, leader
        - alpha2:7080, id: 2
        - alpha3:7080, id: 3
    - predicates
        - dgraph.type
        - counter.val
- Enterprise license
    - Enabled
    - maxNodes: unlimited
    - License expires on Friday, January 10, 2020 4:59:27 PM GMT-08:00 (converted from epoch timestamp)
- Other data:
    - maxTxnTs
        - The current max lease of transaction timestamps used to hand out start timestamps
          and commit timestamps.
        - This increments in batches of 10,000 IDs. Once the max lease is reached, another
          10,000 IDs are leased. In the event that the Zero leader is lost, then the new
          leader starts a brand new lease from maxTxnTs+1 . Any lost transaction IDs
          in-between will never be used.
        - An admin can use the Zero endpoint HTTP GET `/assign?what=timestamps&num=1000` to
          increase the current transaction timestamp (in this case, by 1000). This is mainly
          useful in special-case scenarios, e.g., using an existing p directory to a fresh
          cluster in order to be able to query the latest data in the DB.
    - maxLeaseId
        - The current max lease of UIDs used for blank node UID assignment.
        - This increments in batches of 10,000 IDs. Once the max lease is reached, another
          10,000 IDs are leased. In the event that the Zero leader is lost, the new leader
          starts a brand new lease from maxLeaseId+1. Any UIDs lost in-between will never
          be used for blank-node UID assignment.
        - An admin can use the Zero endpoint HTTP GET `/assign?what=uids&num=1000` to
          reserve a range of UIDs (in this case, 1000) to use externally (Zero will NEVER
          use these UIDs for blank node UID assignment, so the user can use the range
          to assign UIDs manually to their own data sets.
    - CID
        - This is a unique UUID representing the *cluster-ID* for this cluster. It is generated
          during the initial DB startup and is retained across restarts.
    - Group checksum
        - This is the checksum verification of the data per Alpha group. This is used internally
          to verify group memberships in the event of a tablet move.

{{% notice "note" %}}
"tablet", "predicate", and "edge" are synonymous terms today. The future plan to
improve data scalability is to shard a predicate into separate tablets that could
be assigned to different groups.
{{% /notice %}}