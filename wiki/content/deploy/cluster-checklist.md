+++
date = "2017-03-20T22:25:17+11:00"
title = "Cluster Checklist"
[menu.main]
    parent = "deploy"
    weight = 11
+++

In setting up a cluster be sure the check the following.

* Is at least one Dgraph Zero node running?
* Is each Dgraph Alpha instance in the cluster set up correctly?
* Will each Dgraph Alpha instance be accessible to all peers on 7080 (+ any port offset)?
* Does each instance have a unique ID on startup?
* Has `--bindall=true` been set for networked communication?

See the [Production Checklist]({{< relref "production-checklist.md" >}}) docs for more info.
