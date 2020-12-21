+++
date = "2017-03-20T22:25:17+11:00"
title = "Transactions: FAQ"
weight = 1
[menu.main]
    parent = "design-concepts"
+++

Dgraph supports distributed ACID transactions through snapshot isolation.

## Can we do pre-writes only on leaders?

Seems like a good idea, but has bad implications. If we only do a prewrite
in-memory, only on leader, then this prewrite wouldn't make it to the Raft log,
or disk; but would be considered successful.

Then zero could mark the transaction as committed; but this leader could go
down, or leadership could change. In such a case, we'd end up losing the
transaction altogether despite it having been considered committed.

Therefore, pre-writes do have to make it to disk. And if so, better to propose
them in a Raft group.