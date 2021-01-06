+++
date = "2017-03-20T22:25:17+11:00"
title = "Single Host Setup"
weight = 4
[menu.main]
    parent = "deploy"
+++

## Run directly on the host

### Run dgraph zero

```sh
dgraph zero --my=IPADDR:5080
```
The `--my` flag is the connection that Dgraph alphas would dial to talk to
zero. So, the port `5080` and the IP address must be visible to all the Dgraph alphas.

For all other various flags, run `dgraph zero --help`.

### Run dgraph alpha

```sh
dgraph alpha --my=IPADDR:7080 --zero=localhost:5080
dgraph alpha --my=IPADDR:7081 --zero=localhost:5080 -o=1
```

Notice the use of `-o` for the second Alpha to add offset to the default ports used. Zero automatically assigns an unique ID to each Alpha, which is persisted in the write ahead log (wal) directory, users can specify the index using `--idx` option. Dgraph Alphas use two directories to persist data and
wal logs, and these directories must be different for each Alpha if they are running on the same host. You can use `-p` and `-w` to change the location of the data and WAL directories. For all other flags, run

`dgraph alpha --help`.

### Run dgraph UI

```sh
dgraph-ratel
```

## Run using Docker

Dgraph cluster can be setup running as containers on a single host. First, you'd want to figure out the host IP address. You can typically do that via

```sh
ip addr  # On Arch Linux
ifconfig # On Ubuntu/Mac
```
We'll refer to the host IP address via `HOSTIPADDR`.

### Create Docker network

```sh
docker network create dgraph_default
```

### Run dgraph zero

```sh
mkdir ~/zero # Or any other directory where data should be stored.

docker run -it -p 5080:5080 --network dgraph_default -p 6080:6080 -v ~/zero:/dgraph dgraph/dgraph:{{< version >}} dgraph zero --my=HOSTIPADDR:5080
```

### Run dgraph alpha
```sh
mkdir ~/server1 # Or any other directory where data should be stored.

docker run -it -p 7080:7080 --network dgraph_default -p 8080:8080 -p 9080:9080 -v ~/server1:/dgraph dgraph/dgraph:{{< version >}} dgraph alpha --zero=HOSTIPADDR:5080 --my=HOSTIPADDR:7080
```
```sh
mkdir ~/server2 # Or any other directory where data should be stored.

docker run -it -p 7081:7081 --network dgraph_default -p 8081:8081 -p 9081:9081 -v ~/server2:/dgraph dgraph/dgraph:{{< version >}} dgraph alpha --zero=HOSTIPADDR:5080 --my=HOSTIPADDR:7081  -o=1
```
Notice the use of -o for server2 to override the default ports for server2.

### Run dgraph UI
```sh
docker run -it -p 8000:8000 --network dgraph_default dgraph/dgraph:{{< version >}} dgraph-ratel
```

## Run using Docker Compose (On single AWS instance)

We will use [Docker Machine](https://docs.docker.com/machine/overview/). It is a tool that lets you install Docker Engine on virtual machines and easily deploy applications.

* [Install Docker Machine](https://docs.docker.com/machine/install-machine/) on your machine.

{{% notice "note" %}}These instructions are for running Dgraph Alpha without TLS config.
Instructions for running with TLS refer [TLS instructions]({{< relref "deploy/tls-configuration.md" >}}).{{% /notice %}}

Here we'll go through an example of deploying Dgraph Zero, Alpha and Ratel on an AWS instance.

* Make sure you have Docker Machine installed by following [instructions](https://docs.docker.com/machine/install-machine/), provisioning an instance on AWS is just one step away. You'll have to [configure your AWS credentials](http://docs.aws.amazon.com/sdk-for-java/v1/developer-guide/setup-credentials.html) for programmatic access to the Amazon API.

* Create a new docker machine.

```sh
docker-machine create --driver amazonec2 aws01
```

Your output should look like

```sh
Running pre-create checks...
Creating machine...
(aws01) Launching instance...
...
...
Docker is up and running!
To see how to connect your Docker Client to the Docker Engine running on this virtual machine, run: docker-machine env aws01
```

The command would provision a `t2-micro` instance with a security group called `docker-machine`
(allowing inbound access on 2376 and 22). You can either edit the security group to allow inbound access to '5080`, `8080`, `9080` (default ports for Dgraph Zero & Alpha) or you can provide your own security
group which allows inbound access on port 22, 2376 (required by Docker Machine), 5080, 8080 and 9080. Remember port *5080* is only required if you are running Dgraph Live Loader or Dgraph Bulk Loader from outside.

[Here](https://docs.docker.com/machine/drivers/aws/#options) is a list of full options for the `amazonec2` driver which allows you choose the instance type, security group, AMI among many other things.

{{% notice "tip" %}}Docker machine supports [other drivers](https://docs.docker.com/machine/drivers/gce/) like GCE, Azure etc.{{% /notice %}}

* Install and run Dgraph using docker-compose

Docker Compose is a tool for running multi-container Docker applications. You can follow the
instructions [here](https://docs.docker.com/compose/install/) to install it.

Run the command below to download the `docker-compose.yml` file on your machine.

```sh
wget https://github.com/dgraph-io/dgraph/raw/master/contrib/config/docker/docker-compose.yml
```

{{% notice "note" %}}The config mounts `/data`(you could mount something else) on the instance to `/dgraph` within the
container for persistence.{{% /notice %}}

* Connect to the Docker Engine running on the machine.

Running `docker-machine env aws01` tells us to run the command below to configure
our shell.
```
eval $(docker-machine env aws01)
```
This configures our Docker client to talk to the Docker engine running on the AWS Machine.

Finally run the command below to start the Zero and Alpha.
```
docker-compose up -d
```
This would start 3 Docker containers running Dgraph Zero, Alpha and Ratel on the same machine. Docker would restart the containers in case there is any error.
You can look at the logs using `docker-compose logs`.
