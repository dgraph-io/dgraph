+++
date = "2020-31-08T19:35:35+11:00"
title = "Connection"
[menu.main]
    parent = "ratel"
    weight = 1
+++

## Recent Servers

List of recent clusters conected. You can select any of the list to connect. It also has an icon which indicates the version of the cluster running.

Green icon: Running the latest version. <br>
Yellow icon: Running X. <br>
Red icon: No connection found. <br>
Delete icon: Remove the address form the list. <br>

## URL Input box

In this box you add a valid Dgraph Alpha address. You just need to click on "Connect" that Ratel will be connected with the cluster.

> Note: Some users confuses at this point. To connect to a basic Dgraph instance, you just need to click on "Connect" nothing else. There are places to login to ACL. Which is an enterpreise feature, if you're not using it, you don't need to login. After all green, finnaly click in "Continue" button.

bellow the input box you have tree icons which gives you status of the connection.

Network Access: Uses a "Energy Plug" icon. <br>
Server Health: Uses a "Heart" icon. <br>
Logging in: a "lock" icon. <br>

## Cluster Settings

### ACL Account

The ACL Account login is necessary only when you have ACL feature enabled.
The default password for a cluster started from scratch is "password" and the user is "groot".

### Dgraph Zero

If you use an unusual address for Zero instance, you should inform here.

### Extra Settings

Query timeout (seconds): This is a timeout for queries and mutations. If the operation takes too long, it will be dropped in X seconds in the cluster.

Slash API Key: Used to access Slash services.