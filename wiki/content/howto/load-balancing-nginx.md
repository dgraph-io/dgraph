+++
date = "2017-03-20T22:25:17+11:00"
title = "Load balancing queries with Nginx"
weight = 8
[menu.main]
    parent = "howto"
+++

There might be times when you'll want to set up a load balancer to accomplish goals such as increasing the utilization of your database by sending queries from the app to multiple database server replicas. You can follow these steps to get started with that.

## Download ZIP

Download the contents of this gist's ZIP file and extract it to a directory called `graph-nginx`

```
mkdir dgraph-nginx
cd dgraph-nginx
wget -O dgraph-nginx.zip https://gist.github.com/danielmai/0cf7647b27c7626ad8944c4245a9981e/archive/5a2f1a49ca2f77bc39981749e4783e3443eb3ad9.zip
unzip -j dgraph-nginx.zip
```
Two files will be created: `docker-compose.yml` and `nginx.conf`.

## Start Dgraph cluster

Start a 6-node Dgraph cluster (3 Dgraph Zero, 3 Dgraph Alpha, replication setting 3) by starting the Docker Compose config:

```
docker-compose up
```
## Use the increment tool to start a gRPC LB

In a different shell, run the `dgraph increment` [docs](https://dgraph.io/docs/howto/#using-the-increment-tool) tool against the Nginx gRPC load balancer (`nginx:9080`):

```
docker-compose exec alpha1 dgraph increment --alpha nginx:9080 --num=10
```
If you have `dgraph` installed on your host machine, then you can also run this from the host:

```
dgraph increment --alpha localhost:9080 --num=10
```
The increment tool uses the Dgraph Go client to establish a gRPC connection against the `--alpha` flag and transactionally increments a counter predicate `--num` times.

## Check logs

In the Nginx access logs (in the docker-compose up shell window), you'll see access logs like the following:

{{% notice "note" %}}
It is important to take into account with gRPC load balancing that every request hits a different Alpha, potentially increasing read throughput.
{{% /notice %}}

```
nginx_1   | [15/Jan/2020:03:12:02 +0000] 172.20.0.9 - - -  nginx to: 172.20.0.7:9080: POST /api.Dgraph/Query HTTP/2.0 200 upstream_response_time 0.008 msec 1579057922.135 request_time 0.009
nginx_1   | [15/Jan/2020:03:12:02 +0000] 172.20.0.9 - - -  nginx to: 172.20.0.2:9080: POST /api.Dgraph/Query HTTP/2.0 200 upstream_response_time 0.012 msec 1579057922.149 request_time 0.013
nginx_1   | [15/Jan/2020:03:12:02 +0000] 172.20.0.9 - - -  nginx to: 172.20.0.5:9080: POST /api.Dgraph/Query HTTP/2.0 200 upstream_response_time 0.008 msec 1579057922.162 request_time 0.012
nginx_1   | [15/Jan/2020:03:12:02 +0000] 172.20.0.9 - - -  nginx to: 172.20.0.7:9080: POST /api.Dgraph/Query HTTP/2.0 200 upstream_response_time 0.012 msec 1579057922.176 request_time 0.013
nginx_1   | [15/Jan/2020:03:12:02 +0000] 172.20.0.9 - - -  nginx to: 172.20.0.2:9080: POST /api.Dgraph/Query HTTP/2.0 200 upstream_response_time 0.012 msec 1579057922.188 request_time 0.011
nginx_1   | [15/Jan/2020:03:12:02 +0000] 172.20.0.9 - - -  nginx to: 172.20.0.5:9080: POST /api.Dgraph/Query HTTP/2.0 200 upstream_response_time 0.016 msec 1579057922.202 request_time 0.013
```
These logs show that traffic os being load balanced to the following upstream addresses defined in alpha_grpc in nginx.conf:

- `nginx to: 172.20.0.7`
- `nginx to: 172.20.0.2`
- `nginx to: 172.20.0.5`

By default, Nginx load balancing is done round-robin.