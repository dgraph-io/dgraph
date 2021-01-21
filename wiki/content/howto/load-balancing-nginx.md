+++
date = "2017-03-20T22:25:17+11:00"
title = "Load balancing queries with NGINX"
weight = 8
[menu.main]
    parent = "howto"
+++

There might be times when you'll want to set up a load balancer to accomplish goals such as increasing the utilization of your database by sending queries from the app to multiple database server replicas. You can follow these steps to get started with that.

## Setting up NGINX load balancer using Docker Compose

### Dowload ZIP

Download the contents of this gist's ZIP file and extract it to a directory called `graph-nginx`, as follows:

```sh
mkdir dgraph-nginx
cd dgraph-nginx
wget -O dgraph-nginx.zip https://gist.github.com/danielmai/0cf7647b27c7626ad8944c4245a9981e/archive/5a2f1a49ca2f77bc39981749e4783e3443eb3ad9.zip
unzip -j dgraph-nginx.zip
```
Two files will be created: `docker-compose.yml` and `nginx.conf`.

### Start Dgraph cluster

Start a 6-node Dgraph cluster (3 Dgraph Zero, 3 Dgraph Alpha, replication setting 3) by starting the Docker Compose config:

```sh
docker-compose up
```

## Setting up NGINX load balancer with Dgraph running directly on the host machine

You can start your Dgraph cluster directly on the host machine (for example, with systemd) as follows:

### Install NGINX using the following `apt-get` command:

After you have set up your Dgraph cluster, install the latest stable nginx. On Debian and Ubuntu systems use the following command:
```sh
apt-get install nginx
```
### Configure NGINX as a load balancer 

Make sure that your Dgraph cluster is up and running (it this case we will refer to a 6 node cluster). After installing NGINX, you can configure it for load balancing. You do this by specifying which types of connections to listen to, and where to redirect them. Create a new configuration file called `load-balancer.conf`:

```sh
sudo vim /etc/nginx/conf.d/load-balancer.conf
```

and edit it to read as follows:

```sh
upstream alpha_grpc {
  server alpha1:9080;
  server alpha2:9080;
  server alpha3:9080;
}

upstream alpha_http {
  server alpha1:8080;
  server alpha2:8080;
  server alpha3:8080;
}

# $upstream_addr is the ip:port of the Dgraph Alpha defined in the upstream
# Example: 172.25.0.2, 172.25.0.7, 172.25.0.5 are the IP addresses of alpha1, alpha2, and alpha3
# /var/log/nginx/access.log will contain these logs showing "localhost to <upstream address>"
# for the different backends. By default, NGINX load balancing is round robin.

log_format upstreamlog '[$time_local] $remote_addr - $remote_user - $server_name $host to: $upstream_addr: $request $status upstream_response_time $upstream_response_time msec $msec request_time $request_time';

server {
  listen 9080 http2;
  access_log /var/log/nginx/access.log upstreamlog;
  location / {
    grpc_pass grpc://alpha_grpc;
  }
}

server {
  listen 8080;
  access_log /var/log/nginx/access.log upstreamlog;
  location / {
    proxy_pass http://alpha_http;
  }
}
```

Next, disable the default server configuration; on Debian and Ubuntu systems you’ll need to remove the default symbolic link from the **sites-enabled** folder.

```sh
rm /etc/nginx/sites-enabled/default
```

Now you can restart `nginx`:

```sh
systemctl restart nginx
```

## Use the increment tool to start a gRPC LB

In a different shell, run the `dgraph increment` ([docs](https://dgraph.io/docs/howto/#using-the-increment-tool)) tool against the NGINX gRPC load balancer (`nginx:9080`):

```sh
docker-compose exec alpha1 dgraph increment --alpha nginx:9080 --num=10
```

If you have Dgraph installed on your host machine, then you can also run this from the host:

```sh
dgraph increment --alpha localhost:9080 --num=10
```

The increment tool uses the Dgraph Go client to establish a gRPC connection against the `--alpha` flag and transactionally increments a counter predicate `--num` times.

## Check logs

In the NGINX access logs (in the `docker-compose` up shell window), or if you are not using Docker Compose you can tail logs from `/var/log/nginx/access.log`. You'll see access logs like the following:

{{% notice "note" %}}
With gRPC load balancing, each request can hit a different Alpha node. This can increase read throughput.
{{% /notice %}}

```sh
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

## Load balancing methods

By default, NGINX load balancing is done round-robin. By the way There are other load-balancing methods available such as least connections or IP hashing. To use a different method than round-robin, specify the desired load-balancing method in the upstream section of `load-balancer.conf`.

```sh
# use least connection method
upstream alpha_grpc {
  least_conn;
  server alpha1:9080;
  server alpha2:9080;
  server alpha3:9080;
}

upstream alpha_http {
  least_conn;
  server alpha1:8080;
  server alpha2:8080;
  server alpha3:8080;
}
```
