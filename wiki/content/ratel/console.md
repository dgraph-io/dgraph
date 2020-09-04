+++
date = "2020-31-08T19:35:35+11:00"
title = "Console"
[menu.main]
    parent = "ratel"
    weight = 2
+++

## Query Panel

You have two options to run two kinds of operations. Queries and Mutations. The history section holds either queries or mutations.

### Query

On this panel, you can run only DQL (former GraphQL+-). You can use # to comment on something.
You have also the DQL Variable see more at ###


### Mutation

On this panel, you can run RDF mutations and JSON.

## Result Panel

### Graph

On this tab, you will view the query result in a Graph format. Which you can visualize the Nodes and their relations.

### JSON

On this tab, you have the JSON response from the cluster. The actual data comes in the key "data". You have also key "extensions" which returns server_latency, txn, and other metrics.

### Request

On this tab, you have the actual request sent to the cluster.

### Geo

On this tab, you can visualize a query that provides Geodata.
Your objects must contain a predicate or alias named 'location' to use the geo display.
To show a label, use a predicate or alias named 'name'.