+++
date = "2017-03-20T22:25:17+11:00"
title = "Deploy"
+++

This page talks about running Dgraph in various deployment modes, in a distributed fashion and involves
running multiple instances of Dgraph, over multiple servers in a cluster.

{{% notice "tip" %}}
For a single server setup, recommended for new users, please see [Get Started]({{< relref "get-started/index.md" >}}) page.
{{% /notice %}}

## Install Dgraph

### Docker

```sh
docker pull dgraph/dgraph:latest

# You can test that it worked fine, by running:
docker run -it dgraph/dgraph:latest dgraph
```

### Automatic download

Running

```sh
curl https://get.dgraph.io -sSf | bash

# Test that it worked fine, by running:
dgraph
```

would install the `dgraph` binary into your system.

Other instalation options:

> Add `-s --` before the flags.

`-y | --accept-license`: Automatically agree to the terms of the Dgraph Community License (default: "n").

`-s | --systemd`: Automatically create Dgraph's installation as Systemd services (default: "n").

`-v | --version`: Choose Dgraph's version manually (default: The latest stable release, you can do tag combinations e.g v2.0.0-beta1 or -rc1).

>Installing Dgraph and requesting the automatic creation of systemd service. e.g:

```sh
curl https://get.dgraph.io -sSf | bash -s -- --systemd
```

Using Environment variables:

`ACCEPT_LICENSE`: Automatically agree to the terms of the Dgraph Community License (default: "n").

`INSTALL_IN_SYSTEMD`: Automatically create Dgraph's installation as Systemd services (default: "n").

`VERSION`: Choose Dgraph's version manually (default: The latest stable release).

```sh
curl https://get.dgraph.io -sSf | VERSION=v2.0.0-beta1 bash
```

{{% notice "note" %}}
Be aware that using this script will overwrite the installed version and can lead to compatibility problems. For example, if you were using version v1.0.5 and forced the installation of v2.0.0-Beta, the existing data won't be compatible with the new version. The data must be [exported]({{< relref "deploy/index.md#exporting-database" >}}) before running this script and reimported to the new cluster running the updated version.
{{% /notice %}}

### Manual download [optional]

If you don't want to follow the automatic installation method, you could manually download the appropriate tar for your platform from **[Dgraph releases](https://github.com/dgraph-io/dgraph/releases)**. After downloading the tar for your platform from Github, extract the binary to `/usr/local/bin` like so.

```sh
# For Linux
$ sudo tar -C /usr/local/bin -xzf dgraph-linux-amd64-VERSION.tar.gz

# For Mac
$ sudo tar -C /usr/local/bin -xzf dgraph-darwin-amd64-VERSION.tar.gz

# Test that it worked fine, by running:
dgraph
```

### Building from Source

{{% notice "note" %}}
You can build the Ratel UI from source seperately following its build
[instructions](https://github.com/dgraph-io/ratel/blob/master/INSTRUCTIONS.md).
Ratel UI is distributed via Dgraph releases using any of the download methods
listed above.
{{% /notice %}}

Make sure you have [Go](https://golang.org/dl/) v1.11+ installed.

You'll need the following dependencies to install Dgraph using `make`:
```bash
sudo apt-get update
sudo apt-get install gcc make
```

After installing Go, run
```sh
# This should install dgraph binary in your $GOPATH/bin.

git clone https://github.com/dgraph-io/dgraph.git
cd ./dgraph
make install
```

If you get errors related to `grpc` while building them, your
`go-grpc` version might be outdated. We don't vendor in `go-grpc`(because it
causes issues while using the Go client). Update your `go-grpc` by running.
```sh
go get -u -v google.golang.org/grpc
```

### Config

The full set of dgraph's configuration options (along with brief descriptions)
can be viewed by invoking dgraph with the `--help` flag. For example, to see
the options available for `dgraph alpha`, run `dgraph alpha --help`.

The options can be configured in multiple ways (from highest precedence to
lowest precedence):

- Using command line flags (as described in the help output).

- Using environment variables.

- Using a configuration file.

If no configuration for an option is used, then the default value as described
in the `--help` output applies.

Multiple configuration methods can be used all at the same time. E.g. a core
set of options could be set in a config file, and instance specific options
could be set using environment vars or flags.

The environment variable names mirror the flag names as seen in the `--help`
output. They are the concatenation of `DGRAPH`, the subcommand invoked
(`ALPHA`, `ZERO`, `LIVE`, or `BULK`), and then the name of the flag (in
uppercase). For example, instead of using `dgraph alpha --lru_mb=8096`, you
could use `DGRAPH_ALPHA_LRU_MB=8096 dgraph alpha`.

Configuration file formats supported are JSON, TOML, YAML, HCL, and Java
properties (detected via file extension). The file extensions are .json, .toml,
.yml or .yaml, .hcl, and .properties for each format.

A configuration file can be specified using the `--config` flag, or an
environment variable. E.g. `dgraph zero --config my_config.json` or
`DGRAPH_ZERO_CONFIG=my_config.json dgraph zero`.

The config file structure is just simple key/value pairs (mirroring the flag
names).

Example JSON config file (config.json):

```json
{
  "my": "localhost:7080",
  "zero": "localhost:5080",
  "lru_mb": 4096,
  "postings": "/path/to/p",
  "wal": "/path/to/w"
}
```

Example TOML config file (config.toml):

```toml
my = "localhost:7080"
zero = "localhost:5080"
lru_mb = 4096
postings = "/path/to/p"
wal = "/path/to/w"
```


Example YAML config file (config.yml):

```yaml
my: "localhost:7080"
zero: "localhost:5080"
lru_mb: 4096
postings: "/path/to/p"
wal: "/path/to/w"
```

Example HCL config file (config.hcl):

```hcl
my = "localhost:7080"
zero = "localhost:5080"
lru_mb = 4096
postings = "/path/to/p"
wal = "/path/to/w"
```

Example Java properties config file (config.properties):
```text
my=localhost:7080
zero=localhost:5080
lru_mb=4096
postings=/path/to/p
wal=/path/to/w
```

## Cluster Setup

### Understanding Dgraph cluster

Dgraph is a truly distributed graph database - not a master-slave replication of
universal dataset. It shards by predicate and replicates predicates across the
cluster, queries can be run on any node and joins are handled over the
distributed data.  A query is resolved locally for predicates the node stores,
and via distributed joins for predicates stored on other nodes.

For effectively running a Dgraph cluster, it's important to understand how
sharding, replication and rebalancing works.

**Sharding**

Dgraph colocates data per predicate (* P *, in RDF terminology), thus the
smallest unit of data is one predicate. To shard the graph, one or many
predicates are assigned to a group. Each Alpha node in the cluster serves a
single group. Dgraph Zero assigns a group to each Alpha node.

**Shard rebalancing**

Dgraph Zero tries to rebalance the cluster based on the disk usage in each
group. If Zero detects an imbalance, it would try to move a predicate along with
its indices to a group that has minimum disk usage. This can make the predicate
temporarily read-only. Queries for the predicate will still be serviced, but any
mutations for the predicate will be rejected and should be retried after the
move is finished.

Zero would continuously try to keep the amount of data on each server even,
typically running this check on a 10-min frequency.  Thus, each additional
Dgraph Alpha instance would allow Zero to further split the predicates from
groups and move them to the new node.

**Consistent Replication**

If `--replicas` flag is set to something greater than one, Zero would assign the
same group to multiple nodes. These nodes would then form a Raft group aka
quorum. Every write would be consistently replicated to the quorum. To achieve
consensus, its important that the size of quorum be an odd number. Therefore, we
recommend setting `--replicas` to 1, 3 or 5 (not 2 or 4). This allows 0, 1, or 2
nodes serving the same group to be down, respectively without affecting the
overall health of that group.

## Ports Usage

Dgraph cluster nodes use different ports to communicate over gRPC and HTTP. Users should pay attention while choosing these ports based on their topology and deployment-mode as each port needs different access security rules or firewall.

### Types of ports

- **gRPC-internal:** Port that is used between the cluster nodes for internal communication and message exchange.
- **gRPC-external:** Port that is used by Dgraph clients, Dgraph Live Loader , and Dgraph Bulk loader to access APIs over gRPC.
- **http-external:** Port that is used by clients to access APIs over HTTP and other monitoring & administrative tasks.

### Ports used by different nodes

 Dgraph Node Type |     gRPC-internal     | gRPC-external | HTTP-external
------------------|-----------------------|---------------|---------------
       zero       |      5080<sup>1</sup> | --Not Used--  |  6080<sup>2</sup>
       alpha      |      7080             |     9080      |  8080
       ratel      |  --Not Used--         | --Not Used--  |  8000


<sup>1</sup>: Dgraph Zero's gRPC-internal port is used for internal communication within the cluster. It's also needed for the [fast data loading]({{< relref "#fast-data-loading" >}}) tools Dgraph Live Loader and Dgraph Bulk Loader.

<sup>2</sup>: Dgraph Zero's HTTP-external port is used for [admin]({{< relref "#more-about-dgraph-zero" >}}) operations. Access to it is not required by clients.

Users have to modify security rules or open firewall ports depending up on their underlying network to allow communication between cluster nodes and between the Dgraph instances themselves and between Dgraph and a client. A general rule is to make *-external (gRPC/HTTP) ports wide open to clients and gRPC-internal ports open within the cluster nodes.

**Ratel UI** accesses Dgraph Alpha on the HTTP-external port (default localhost:8080) and can be configured to talk to remote Dgraph cluster. This way you can run Ratel on your local machine and point to a remote cluster. But if you are deploying Ratel along with Dgraph cluster, then you may have to expose 8000 to the public.

**Port Offset** To make it easier for user to setup the cluster, Dgraph defaults the ports used by Dgraph nodes and let user to provide an offset  (through command option `--port_offset`) to define actual ports used by the node. Offset can also be used when starting multiple zero nodes in a HA setup.

For example, when a user runs a Dgraph Alpha by setting `--port_offset 2`, then the Alpha node binds to 7082 (gRPC-internal), 8082 (HTTP-external) & 9082 (gRPC-external) respectively.

**Ratel UI** by default listens on port 8000. You can use the `-port` flag to configure to listen on any other port.

### HA Cluster Setup

In a high-availability setup, we need to run 3 or 5 replicas for Zero, and similarly, 3 or 5 replicas for Alpha.
{{% notice "note" %}}
If number of replicas is 2K + 1, up to **K servers** can be down without any impact on reads or writes.

Avoid keeping replicas to 2K (even number). If K servers go down, this would block reads and writes, due to lack of consensus.
{{% /notice %}}

**Dgraph Zero**
Run three Zero instances, assigning a unique ID(Integer) to each via `--idx` flag, and
passing the address of any healthy Zero instance via `--peer` flag.

To run three replicas for the alphas, set `--replicas=3`. Every time a new
Dgraph Alpha is added, Zero would check the existing groups and assign them to
one, which doesn't have three replicas.

**Dgraph Alpha**
Run as many Dgraph Alphas as you want. You can manually set `--idx` flag, or you
can leave that flag empty, and Zero would auto-assign an id to the Alpha. This
id would get persisted in the write-ahead log, so be careful not to delete it.

The new Alphas will automatically detect each other by communicating with
Dgraph zero and establish connections to each other. You can provide a list of
zero addresses to alpha using the `--zero` flag. Alpha will try to connect to
one of the zeros starting from the first zero address in the list. For example:
`--zero=zero1,zero2,zero3` where zero1 is the `host:port` of a zero instance.

Typically, Zero would first attempt to replicate a group, by assigning a new
Dgraph alpha to run the same group as assigned to another. Once the group has
been replicated as per the `--replicas` flag, Zero would create a new group.

Over time, the data would be evenly split across all the groups. So, it's
important to ensure that the number of Dgraph alphas is a multiple of the
replication setting. For e.g., if you set `--replicas=3` in Zero, then run three
Dgraph alphas for no sharding, but 3x replication. Run six Dgraph alphas, for
sharding the data into two groups, with 3x replication.

## Single Host Setup

### Run directly on the host

**Run dgraph zero**

```sh
dgraph zero --my=IPADDR:5080
```
The `--my` flag is the connection that Dgraph alphas would dial to talk to
zero. So, the port `5080` and the IP address must be visible to all the Dgraph alphas.

For all other various flags, run `dgraph zero --help`.

**Run dgraph alpha**

```sh
dgraph alpha --lru_mb=<typically one-third the RAM> --my=IPADDR:7080 --zero=localhost:5080
dgraph alpha --lru_mb=<typically one-third the RAM> --my=IPADDR:7081 --zero=localhost:5080 -o=1
```

Notice the use of `-o` for the second Alpha to add offset to the default ports used. Zero automatically assigns an unique ID to each Alpha, which is persisted in the write ahead log (wal) directory, users can specify the index using `--idx` option. Dgraph Alphas use two directories to persist data and
wal logs, and these directories must be different for each Alpha if they are running on the same host. You can use `-p` and `-w` to change the location of the data and WAL directories. For all other flags, run

`dgraph alpha --help`.

**Run dgraph UI**

```sh
dgraph-ratel
```

### Run using Docker

Dgraph cluster can be setup running as containers on a single host. First, you'd want to figure out the host IP address. You can typically do that via

```sh
ip addr  # On Arch Linux
ifconfig # On Ubuntu/Mac
```
We'll refer to the host IP address via `HOSTIPADDR`.

**Create Docker network**

```sh
docker network create dgraph_default
```

**Run dgraph zero**

```sh
mkdir ~/zero # Or any other directory where data should be stored.

docker run -it -p 5080:5080 --network dgraph_default -p 6080:6080 -v ~/zero:/dgraph dgraph/dgraph:latest dgraph zero --my=HOSTIPADDR:5080
```

**Run dgraph alpha**
```sh
mkdir ~/server1 # Or any other directory where data should be stored.

docker run -it -p 7080:7080 --network dgraph_default -p 8080:8080 -p 9080:9080 -v ~/server1:/dgraph dgraph/dgraph:latest dgraph alpha --lru_mb=<typically one-third the RAM> --zero=HOSTIPADDR:5080 --my=HOSTIPADDR:7080
```
```sh
mkdir ~/server2 # Or any other directory where data should be stored.

docker run -it -p 7081:7081 --network dgraph_default -p 8081:8081 -p 9081:9081 -v ~/server2:/dgraph dgraph/dgraph:latest dgraph alpha --lru_mb=<typically one-third the RAM> --zero=HOSTIPADDR:5080 --my=HOSTIPADDR:7081  -o=1
```
Notice the use of -o for server2 to override the default ports for server2.

**Run dgraph UI**
```sh
docker run -it -p 8000:8000 --network dgraph_default dgraph/dgraph:latest dgraph-ratel
```

### Run using Docker Compose (On single AWS instance)

We will use [Docker Machine](https://docs.docker.com/machine/overview/). It is a tool that lets you install Docker Engine on virtual machines and easily deploy applications.

* [Install Docker Machine](https://docs.docker.com/machine/install-machine/) on your machine.

{{% notice "note" %}}These instructions are for running Dgraph Alpha without TLS config.
Instructions for running with TLS refer [TLS instructions](#tls-configuration).{{% /notice %}}

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
(allowing inbound access on `2376` and `22`). You can either edit the security group to allow inbound access to `5080`, `8080`, `9080` (default ports for Dgraph Zero & Alpha) or you can provide your own security
group which allows inbound access on port `22`, `2376` (required by Docker Machine), `5080`, `8080` and `9080`. Remember port *`5080`* is only required if you are running Dgraph Live Loader or Dgraph Bulk Loader from outside.

{{% notice "note" %}}Dgraph recommended memory is 7-8Gb. A `t2-micro` (free-tier eligible) is only provisioned with 1Gb and will not support larger schemas or data sets. We recommend using a `t2-large` (not free-tier eligible) or larger for a production server.{{% /notice %}}

[Here](https://docs.docker.com/machine/drivers/aws/#options) is a list of full options for the `amazonec2` driver which allows you choose the instance type, security group, AMI among many other things.

{{% notice "tip" %}}Docker machine supports [other drivers](https://docs.docker.com/machine/drivers/gce/) like GCE, Azure etc.{{% /notice %}}

* Install and run Dgraph using docker-compose

Docker Compose is a tool for running multi-container Docker applications. You can follow the
instructions [here](https://docs.docker.com/compose/install/) to install it.

Run the command below to download the `docker-compose.yml` file on your machine.

```sh
wget https://github.com/dgraph-io/dgraph/raw/master/contrib/config/docker/docker-compose.yml
```

{{% notice "note" %}}The config mounts `/tmp/data` on the instance to `/dgraph` within the
container for persistence. You will need to change this to a different directory to persists data as the `tmp` directory is only for illustration purposes and will not persist in that directory.{{% /notice %}}

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

## Multi Host Setup

### Using Docker Swarm

#### Cluster Setup Using Docker Swarm

{{% notice "note" %}}These instructions are for running Dgraph Alpha without TLS config.
Instructions for running with TLS refer [TLS instructions](#tls-configuration).{{% /notice %}}

Here we'll go through an example of deploying 3 Dgraph Alpha nodes and 1 Zero on three different AWS instances using Docker Swarm with a replication factor of 3.

* Make sure you have Docker Machine installed by following [instructions](https://docs.docker.com/machine/install-machine/).

```sh
docker-machine --version
```

* Create 3 instances on AWS and [install Docker Engine](https://docs.docker.com/engine/installation/) on them. This can be done manually or by using `docker-machine`.
You'll have to [configure your AWS credentials](http://docs.aws.amazon.com/sdk-for-java/v1/developer-guide/setup-credentials.html) to create the instances using Docker Machine.

Considering that you have AWS credentials setup, you can use the below commands to start 3 AWS
`t2-medium` instances with Docker Engine installed on them.

```sh
docker-machine create --driver amazonec2 --amazonec2-instance-type t2.medium aws01
docker-machine create --driver amazonec2 --amazonec2-instance-type t2.medium aws02
docker-machine create --driver amazonec2 --amazonec2-instance-type t2.medium aws03
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

The command would provision a `t2-medium` instance with a security group called `docker-machine`
(allowing inbound access on 2376 and 22).

You would need to edit the `docker-machine` security group to open inbound traffic on the following ports.

1. Allow all inbound traffic on all ports with Source being `docker-machine`
   security ports so that Docker related communication can happen easily.

2. Also open inbound TCP traffic on the following ports required by Dgraph:
   `5080`, `6080`, `8000`, `808[0-2]`, `908[0-2]`. Remember port *5080* is only
   required if you are running Dgraph Live Loader or Dgraph Bulk Loader from
   outside. You need to open `7080` to enable Alpha-to-Alpha communication in
   case you have not opened all ports in #1.

If you are on AWS, below is the security group (**docker-machine**) after
necessary changes.

{{% load-img "/images/aws.png" "AWS Security Group" %}}

[Here](https://docs.docker.com/machine/drivers/aws/#options) is a list of full options for the `amazonec2` driver which allows you choose the
instance type, security group, AMI among many other
things.

{{% notice "tip" %}}Docker machine supports [other drivers](https://docs.docker.com/machine/drivers/gce/) like GCE, Azure etc.{{% /notice %}}

Running `docker-machine ls` shows all the AWS EC2 instances that we started.
```sh
➜  ~ docker-machine ls
NAME    ACTIVE   DRIVER       STATE     URL                         SWARM   DOCKER        ERRORS
aws01   -        amazonec2    Running   tcp://34.200.239.30:2376            v17.11.0-ce
aws02   -        amazonec2    Running   tcp://54.236.58.120:2376            v17.11.0-ce
aws03   -        amazonec2    Running   tcp://34.201.22.2:2376              v17.11.0-ce
```

* Start the Swarm

Docker Swarm has manager and worker nodes. Swarm can be started and updated on manager nodes. We
   will setup `aws01` as swarm manager. You can first run the following commands to initialize the
   swarm.

We are going to use the internal IP address given by AWS. Run the following command to get the
internal IP for `aws01`. Lets assume `172.31.64.18` is the internal IP in this case.
```
docker-machine ssh aws01 ifconfig eth0
```

Now that we have the internal IP, let's initiate the Swarm.

```sh
# This configures our Docker client to talk to the Docker engine running on the aws01 host.
eval $(docker-machine env aws01)
docker swarm init --advertise-addr 172.31.64.18
```

Output:
```
Swarm initialized: current node (w9mpjhuju7nyewmg8043ypctf) is now a manager.

To add a worker to this swarm, run the following command:

    docker swarm join \
    --token SWMTKN-1-1y7lba98i5jv9oscf10sscbvkmttccdqtkxg478g3qahy8dqvg-5r5cbsntc1aamsw3s4h3thvgk \
    172.31.64.18:2377

To add a manager to this swarm, run 'docker swarm join-token manager' and follow the instructions.
```

Now we will make other nodes join the swarm.

```sh
eval $(docker-machine env aws02)
docker swarm join \
    --token SWMTKN-1-1y7lba98i5jv9oscf10sscbvkmttccdqtkxg478g3qahy8dqvg-5r5cbsntc1aamsw3s4h3thvgk \
    172.31.64.18:2377
```

Output:
```
This node joined a swarm as a worker.
```

Similarly, aws03
```sh
eval $(docker-machine env aws03)
docker swarm join \
    --token SWMTKN-1-1y7lba98i5jv9oscf10sscbvkmttccdqtkxg478g3qahy8dqvg-5r5cbsntc1aamsw3s4h3thvgk \
    172.31.64.18:2377
```

On the Swarm manager `aws01`, verify that your swarm is running.
```sh
docker node ls
```

Output:

```
ID                            HOSTNAME            STATUS              AVAILABILITY        MANAGER STATUS
ghzapjsto20c6d6l3n0m91zev     aws02               Ready               Active
rb39d5lgv66it1yi4rto0gn6a     aws03               Ready               Active
waqdyimp8llvca9i09k4202x5 *   aws01               Ready               Active              Leader
```

* Start the Dgraph cluster

Run the command below to download the `docker-compose-multi.yml` file on your machine.

```sh
wget https://github.com/dgraph-io/dgraph/raw/master/contrib/config/docker/docker-compose-multi.yml
```

Run the following command on the Swarm leader to deploy the Dgraph Cluster.

```sh
eval $(docker-machine env aws01)
docker stack deploy -c docker-compose-multi.yml dgraph
```

This should run three Dgraph Alpha services (one on each VM because of the
constraint we have), one Dgraph Zero service on aws01 and one Dgraph Ratel.

These placement constraints (as seen in the compose file) are important so that
in case of restarting any containers, swarm places the respective Dgraph Alpha
or Zero containers on the same hosts to re-use the volumes. Also, if you are
running fewer than three hosts, make sure you use either different volumes or
run Dgraph Alpha with `-p p1 -w w1` options.

{{% notice "note" %}}

1. This setup would create and use a local volume called `dgraph_data-volume` on
   the instances. If you plan to replace instances, you should use remote
   storage like
   [cloudstore](https://docs.docker.com/docker-for-aws/persistent-data-volumes)
   instead of local disk. {{% /notice %}}

You can verify that all services were created successfully by running:

```sh
docker service ls
```

Output:

```
ID                NAME               MODE            REPLICAS      IMAGE                     PORTS
vp5bpwzwawoe      dgraph_ratel       replicated      1/1           dgraph/dgraph:latest      *:8000->8000/tcp
69oge03y0koz      dgraph_alpha2      replicated      1/1           dgraph/dgraph:latest      *:8081->8081/tcp,*:9081->9081/tcp
kq5yks92mnk6      dgraph_alpha3      replicated      1/1           dgraph/dgraph:latest      *:8082->8082/tcp,*:9082->9082/tcp
uild5cqp44dz      dgraph_zero        replicated      1/1           dgraph/dgraph:latest      *:5080->5080/tcp,*:6080->6080/tcp
v9jlw00iz2gg      dgraph_alpha1      replicated      1/1           dgraph/dgraph:latest      *:8080->8080/tcp,*:9080->9080/tcp
```

To stop the cluster run:

```sh
docker stack rm dgraph
```

### HA Cluster setup using Docker Swarm

Here is a sample swarm config for running 6 Dgraph Alpha nodes and 3 Zero nodes on 6 different
ec2 instances. Setup should be similar to [Cluster setup using Docker Swarm]({{< relref "#cluster-setup-using-docker-swarm" >}}) apart from a couple of differences. This setup would ensure replication with sharding of data. The file assumes that there are six hosts available as docker-machines. Also if you are running on fewer than six hosts, make sure you use either different volumes or run Dgraph Alpha with `-p p1 -w w1` options.

You would need to edit the `docker-machine` security group to open inbound traffic on the following ports.

1. Allow all inbound traffic on all ports with Source being `docker-machine` security ports so that
   docker related communication can happen easily.

2. Also open inbound TCP traffic on the following ports required by Dgraph: `5080`, `8000`, `808[0-5]`, `908[0-5]`. Remember port *5080* is only required if you are running Dgraph Live Loader or Dgraph Bulk Loader from outside. You need to open `7080` to enable Alpha-to-Alpha communication in case you have not opened all ports in #1.

If you are on AWS, below is the security group (**docker-machine**) after necessary changes.

{{% load-img "/images/aws.png" "AWS Security Group" %}}

Run the command below to download the `docker-compose-ha.yml` file on your machine.

```sh
wget https://github.com/dgraph-io/dgraph/raw/master/contrib/config/docker/docker-compose-ha.yml
```

Run the following command on the Swarm leader to deploy the Dgraph Cluster.

```sh
eval $(docker-machine env aws01)
docker stack deploy -c docker-compose-ha.yml dgraph
```

You can verify that all services were created successfully by running:

```sh
docker service ls
```

Output:
```
ID                NAME               MODE            REPLICAS      IMAGE                     PORTS
qck6v1lacvtu      dgraph_alpha1      replicated      1/1           dgraph/dgraph:latest      *:8080->8080/tcp, *:9080->9080/tcp
i3iq5mwhxy8a      dgraph_alpha2      replicated      1/1           dgraph/dgraph:latest      *:8081->8081/tcp, *:9081->9081/tcp
2ggma86bw7h7      dgraph_alpha3      replicated      1/1           dgraph/dgraph:latest      *:8082->8082/tcp, *:9082->9082/tcp
wgn5adzk67n4      dgraph_alpha4      replicated      1/1           dgraph/dgraph:latest      *:8083->8083/tcp, *:9083->9083/tcp
uzviqxv9fp2a      dgraph_alpha5      replicated      1/1           dgraph/dgraph:latest      *:8084->8084/tcp, *:9084->9084/tcp
nl1j457ko54g      dgraph_alpha6      replicated      1/1           dgraph/dgraph:latest      *:8085->8085/tcp, *:9085->9085/tcp
s11bwr4a6371      dgraph_ratel       replicated      1/1           dgraph/dgraph:latest      *:8000->8000/tcp
vchibvpquaes      dgraph_zero1       replicated      1/1           dgraph/dgraph:latest      *:5080->5080/tcp, *:6080->6080/tcp
199rezd7pw7c      dgraph_zero2       replicated      1/1           dgraph/dgraph:latest      *:5081->5081/tcp, *:6081->6081/tcp
yb8ella56oxt      dgraph_zero3       replicated      1/1           dgraph/dgraph:latest      *:5082->5082/tcp, *:6082->6082/tcp
```

To stop the cluster run:

```sh
docker stack rm dgraph
```

{{% notice "note" %}}
1. This setup assumes that you are using 6 hosts, but if you are running fewer than 6 hosts then you have to either use different volumes between Dgraph alphas or use `-p` & `-w` to configure data directories.
2. This setup would create and use a local volume called `dgraph_data-volume` on the instances. If you plan to replace instances, you should use remote storage like [cloudstore](https://docs.docker.com/docker-for-aws/persistent-data-volumes) instead of local disk. {{% /notice %}}

## Using Kubernetes

The following section covers running Dgraph with Kubernetes v1.8.4.

{{% notice "note" %}}These instructions are for running Dgraph Alpha without TLS config.
Instructions for running with TLS refer [TLS instructions](#tls-configuration).{{% /notice %}}

* Install [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/) which is used to deploy
  and manage applications on kubernetes.
* Get the kubernetes cluster up and running on a cloud provider of your choice. You can use [kops](https://github.com/kubernetes/kops/blob/master/docs/aws.md) to set it up on AWS. Kops does auto-scaling by default on AWS and creates the volumes and instances for you.

Verify that you have your cluster up and running using `kubectl get nodes`. If you used `kops` with
the default options, you should have a master and two worker nodes ready.

```sh
➜  kubernetes git:(master) ✗ kubectl get nodes
NAME                                          STATUS    ROLES     AGE       VERSION
ip-172-20-42-118.us-west-2.compute.internal   Ready     node      1h        v1.8.4
ip-172-20-61-179.us-west-2.compute.internal   Ready     master    2h        v1.8.4
ip-172-20-61-73.us-west-2.compute.internal    Ready     node      2h        v1.8.4
```

### Single Server

Once your Kubernetes cluster is up, you can use [dgraph-single.yaml](https://github.com/dgraph-io/dgraph/blob/master/contrib/config/kubernetes/dgraph-single/dgraph-single.yaml) to start a Zero and Alpha.

* From your machine, run the following command to start a StatefulSet that
  creates a Pod with Zero and Alpha running in it.

```sh
kubectl create -f https://raw.githubusercontent.com/dgraph-io/dgraph/master/contrib/config/kubernetes/dgraph-single/dgraph-single.yaml
```

Output:
```
service "dgraph-public" created
statefulset "dgraph" created
```

* Confirm that the pod was created successfully.

```sh
kubectl get pods
```

Output:
```
NAME       READY     STATUS    RESTARTS   AGE
dgraph-0   3/3       Running   0          1m
```

{{% notice "tip" %}}
You can check the logs for the containers in the pod using
`kubectl logs -f dgraph-0 <container_name>`. For example, try
`kubectl logs -f dgraph-0 alpha` for server logs.
{{% /notice %}}

* Test the setup

Port forward from your local machine to the pod

```sh
kubectl port-forward dgraph-0 8080
kubectl port-forward dgraph-0 8000
```

Go to `http://localhost:8000` and verify Dgraph is working as expected.

{{% notice "note" %}} You can also access the service on its External IP address.{{% /notice %}}


* Stop the cluster

Delete all the resources

```sh
kubectl delete pods,statefulsets,services,persistentvolumeclaims,persistentvolumes -l app=dgraph
```

Stop the cluster. If you used `kops` you can run the following command.

```sh
kops delete cluster ${NAME} --yes
```

### HA Cluster Setup Using Kubernetes

This setup allows you to run 3 Dgraph Alphas and 3 Dgraph Zeros. We start Zero with `--replicas
3` flag, so all data would be replicated on 3 Alphas and form 1 alpha group.

{{% notice "note" %}} Ideally you should have at least three worker nodes as part of your Kubernetes
cluster so that each Dgraph Alpha runs on a separate node.{{% /notice %}}

* Check the nodes that are part of the Kubernetes cluster.

```sh
kubectl get nodes
```

Output:
```sh
NAME                                          STATUS    ROLES     AGE       VERSION
ip-172-20-34-90.us-west-2.compute.internal    Ready     master    6m        v1.8.4
ip-172-20-51-1.us-west-2.compute.internal     Ready     node      4m        v1.8.4
ip-172-20-59-116.us-west-2.compute.internal   Ready     node      4m        v1.8.4
ip-172-20-61-88.us-west-2.compute.internal    Ready     node      5m        v1.8.4
```

Once your Kubernetes cluster is up, you can use [dgraph-ha.yaml](https://github.com/dgraph-io/dgraph/blob/master/contrib/config/kubernetes/dgraph-ha/dgraph-ha.yaml) to start the cluster.

* From your machine, run the following command to start the cluster.

```sh
kubectl create -f https://raw.githubusercontent.com/dgraph-io/dgraph/master/contrib/config/kubernetes/dgraph-ha/dgraph-ha.yaml
```

Output:
```sh
service "dgraph-zero-public" created
service "dgraph-alpha-public" created
service "dgraph-alpha-0-http-public" created
service "dgraph-ratel-public" created
service "dgraph-zero" created
service "dgraph-alpha" created
statefulset "dgraph-zero" created
statefulset "dgraph-alpha" created
deployment "dgraph-ratel" created
```

* Confirm that the pods were created successfully.

```sh
kubectl get pods
```

Output:
```sh
NAME                   READY     STATUS    RESTARTS   AGE
dgraph-ratel-<pod-id>  1/1       Running   0          9s
dgraph-alpha-0         1/1       Running   0          2m
dgraph-alpha-1         1/1       Running   0          2m
dgraph-alpha-2         1/1       Running   0          2m
dgraph-zero-0          1/1       Running   0          2m
dgraph-zero-1          1/1       Running   0          2m
dgraph-zero-2          1/1       Running   0          2m

```

{{% notice "tip" %}}You can check the logs for the containers in the pod using `kubectl logs -f dgraph-alpha-0` and `kubectl logs -f dgraph-zero-0`.{{% /notice %}}

* Test the setup

Port forward from your local machine to the pod

```sh
kubectl port-forward dgraph-alpha-0 8080
kubectl port-forward dgraph-ratel-<pod-id> 8000
```

Go to `http://localhost:8000` and verify Dgraph is working as expected.

{{% notice "note" %}} You can also access the service on its External IP address.{{% /notice %}}


* Stop the cluster

Delete all the resources

```sh
kubectl delete pods,statefulsets,services,persistentvolumeclaims,persistentvolumes -l app=dgraph-zero
kubectl delete pods,statefulsets,services,persistentvolumeclaims,persistentvolumes -l app=dgraph-alpha
kubectl delete pods,replicasets,services,persistentvolumeclaims,persistentvolumes -l app=dgraph-ratel
```

Stop the cluster. If you used `kops` you can run the following command.

```sh
kops delete cluster ${NAME} --yes
```

### Using Helm Chart

Once your Kubernetes cluster is up, you can make use of the Helm chart present
[in our official helm repository here](https://github.com/dgraph-io/charts/) to bring
up a Dgraph cluster.

{{% notice "note" %}}The instructions below are for Helm versions >= 3.x.{{% /notice %}}

#### Installing the Chart

To add the Dgraph helm repository:

```sh
helm repo add dgraph https://charts.dgraph.io
```

To install the chart with the release name `my-release`:

```sh
helm install my-release dgraph/dgraph
```

The above command will install the latest available dgraph docker image. In order to install the older versions:

```sh
helm install my-release dgraph/dgraph --set image.tag="v1.1.0"
```

By default zero and alpha services are exposed only within the kubernetes cluster as
kubernetes service type "ClusterIP". In order to expose the alpha service publicly
you can use kubernetes service type "LoadBalancer":

```sh
helm install my-release dgraph/dgraph --set alpha.service.type="LoadBalancer"
```

Similarly, you can expose alpha and ratel service to the internet as follows:

```sh
helm install my-release dgraph/dgraph --set alpha.service.type="LoadBalancer" --set ratel.service.type="LoadBalancer"
```

#### Upgrading the Chart

You can update your cluster configuration by updating the configuration of the
Helm chart. Dgraph is a stateful database that requires some attention on
upgrading the configuration carefully in order to update your cluster to your
desired configuration.

In general, you can use [`helm upgrade`][helm-upgrade] to update the
configuration values of the cluster. Depending on your change, you may need to
upgrade the configuration in multiple steps following the steps below.

[helm-upgrade]: https://helm.sh/docs/helm/helm_upgrade/

**Upgrade to HA cluster setup**

To upgrade to an [HA cluster setup]({{< relref "#ha-cluster-setup" >}}), ensure
that the shard replication setting is more than 1. When `zero.shardReplicaCount`
is not set to an HA configuration (3 or 5), follow the steps below:

1. Set the shard replica flag on the Zero node group. For example: `zero.shardReplicaCount=3`.
2. Next, run the Helm upgrade command to restart the Zero node group: 
   ```sh
   helm upgrade my-release dgraph/dgraph [options]
   ```
3. Now set the Alpha replica count flag. For example: `alpha.replicaCount=3`.
4. Finally, run the Helm upgrade command again:
   ```sh
   helm upgrade my-release dgraph/dgraph [options]
   ```


#### Deleting the Chart

Delete the Helm deployment as normal

```sh
helm delete my-release
```
Deletion of the StatefulSet doesn't cascade to deleting associated PVCs. To delete them:

```sh
kubectl delete pvc -l release=my-release,chart=dgraph
```

#### Configuration

The following table lists the configurable parameters of the dgraph chart and their default values.

|              Parameter               |                             Description                             |                       Default                       |
| ------------------------------------ | ------------------------------------------------------------------- | --------------------------------------------------- |
| `image.registry`                     | Container registry name                                             | `docker.io`                                         |
| `image.repository`                   | Container image name                                                | `dgraph/dgraph`                                     |
| `image.tag`                          | Container image tag                                                 | `latest`                                            |
| `image.pullPolicy`                   | Container pull policy                                               | `Always`                                            |
| `zero.name`                          | Zero component name                                                 | `zero`                                              |
| `zero.updateStrategy`                | Strategy for upgrading zero nodes                                   | `RollingUpdate`                                     |
| `zero.monitorLabel`                  | Monitor label for zero, used by prometheus.                         | `zero-dgraph-io`                                    |
| `zero.rollingUpdatePartition`        | Partition update strategy                                           | `nil`                                               |
| `zero.podManagementPolicy`           | Pod management policy for zero nodes                                | `OrderedReady`                                      |
| `zero.replicaCount`                  | Number of zero nodes                                                | `3`                                                 |
| `zero.shardReplicaCount`             | Max number of replicas per data shard                               | `5`                                                 |
| `zero.terminationGracePeriodSeconds` | Zero server pod termination grace period                            | `60`                                                |
| `zero.antiAffinity`                  | Zero anti-affinity policy                                           | `soft`                                              |
| `zero.podAntiAffinitytopologyKey`    | Anti affinity topology key for zero nodes                           | `kubernetes.io/hostname`                            |
| `zero.nodeAffinity`                  | Zero node affinity policy                                           | `{}`                                                |
| `zero.service.type`                  | Zero node service type                                              | `ClusterIP`                                         |
| `zero.securityContext.enabled`       | Security context for zero nodes enabled                             | `false`                                             |
| `zero.securityContext.fsGroup`       | Group id of the zero container                                      | `1001`                                              |
| `zero.securityContext.runAsUser`     | User ID for the zero container                                      | `1001`                                              |
| `zero.persistence.enabled`           | Enable persistence for zero using PVC                               | `true`                                              |
| `zero.persistence.storageClass`      | PVC Storage Class for zero volume                                   | `nil`                                               |
| `zero.persistence.accessModes`       | PVC Access Mode for zero volume                                     | `ReadWriteOnce`                                     |
| `zero.persistence.size`              | PVC Storage Request for zero volume                                 | `8Gi`                                               |
| `zero.nodeSelector`                  | Node labels for zero pod assignment                                 | `{}`                                                |
| `zero.tolerations`                   | Zero tolerations                                                    | `[]`                                                |
| `zero.resources`                     | Zero node resources requests & limits                               | `{}`                                                |
| `zero.livenessProbe`                 | Zero liveness probes                                                | `See values.yaml for defaults`                      |
| `zero.readinessProbe`                | Zero readiness probes                                               | `See values.yaml for defaults`                      |
| `alpha.name`                         | Alpha component name                                                | `alpha`                                             |
| `alpha.updateStrategy`               | Strategy for upgrading alpha nodes                                  | `RollingUpdate`                                     |
| `alpha.monitorLabel`                 | Monitor label for alpha, used by prometheus.                        | `alpha-dgraph-io`                                   |
| `alpha.rollingUpdatePartition`       | Partition update strategy                                           | `nil`                                               |
| `alpha.podManagementPolicy`          | Pod management policy for alpha nodes                               | `OrderedReady`                                      |
| `alpha.replicaCount`                 | Number of alpha nodes                                               | `3`                                                 |
| `alpha.terminationGracePeriodSeconds`| Alpha server pod termination grace period                           | `60`                                                |
| `alpha.antiAffinity`                 | Alpha anti-affinity policy                                          | `soft`                                              |
| `alpha.podAntiAffinitytopologyKey`   | Anti affinity topology key for zero nodes                           | `kubernetes.io/hostname`                            |
| `alpha.nodeAffinity`                 | Alpha node affinity policy                                          | `{}`                                                |
| `alpha.service.type`                 | Alpha node service type                                             | `ClusterIP`                                         |
| `alpha.securityContext.enabled`      | Security context for alpha nodes enabled                            | `false`                                             |
| `alpha.securityContext.fsGroup`      | Group id of the alpha container                                     | `1001`                                              |
| `alpha.securityContext.runAsUser`    | User ID for the alpha container                                     | `1001`                                              |
| `alpha.persistence.enabled`          | Enable persistence for alpha using PVC                              | `true`                                              |
| `alpha.persistence.storageClass`     | PVC Storage Class for alpha volume                                  | `nil`                                               |
| `alpha.persistence.accessModes`      | PVC Access Mode for alpha volume                                    | `ReadWriteOnce`                                     |
| `alpha.persistence.size`             | PVC Storage Request for alpha volume                                | `8Gi`                                               |
| `alpha.nodeSelector`                 | Node labels for alpha pod assignment                                | `{}`                                                |
| `alpha.tolerations`                  | Alpha tolerations                                                   | `[]`                                                |
| `alpha.resources`                    | Alpha node resources requests & limits                              | `{}`                                                |
| `alpha.livenessProbe`                | Alpha liveness probes                                               | `See values.yaml for defaults`                      |
| `alpha.readinessProbe`               | Alpha readiness probes                                              | `See values.yaml for defaults`                      |
| `ratel.name`                         | Ratel component name                                                | `ratel`                                             |
| `ratel.replicaCount`                 | Number of ratel nodes                                               | `1`                                                 |
| `ratel.service.type`                 | Ratel service type                                                  | `ClusterIP`                                         |
| `ratel.securityContext.enabled`      | Security context for ratel nodes enabled                            | `false`                                             |
| `ratel.securityContext.fsGroup`      | Group id of the ratel container                                     | `1001`                                              |
| `ratel.securityContext.runAsUser`    | User ID for the ratel container                                     | `1001`                                              |
| `ratel.livenessProbe`                | Ratel liveness probes                                               | `See values.yaml for defaults`                      |
| `ratel.readinessProbe`               | Ratel readiness probes                                              | `See values.yaml for defaults`                      |

### Monitoring in Kubernetes

Dgraph exposes prometheus metrics to monitor the state of various components involved in the cluster, this includes dgraph alpha and zero.

Follow the below mentioned steps to setup prometheus monitoring for your cluster:

* Install Prometheus operator:

```sh
kubectl apply -f https://raw.githubusercontent.com/coreos/prometheus-operator/release-0.34/bundle.yaml
```

* Ensure that the instance of `prometheus-operator` has started before continuing.

```sh
$ kubectl get deployments prometheus-operator
NAME                  DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
prometheus-operator   1         1         1            1           3m
```

* Apply prometheus manifest present [here](https://github.com/dgraph-io/dgraph/blob/master/contrib/config/monitoring/prometheus/prometheus.yaml).

```sh
$ kubectl apply -f prometheus.yaml

serviceaccount/prometheus-dgraph-io created
clusterrole.rbac.authorization.k8s.io/prometheus-dgraph-io created
clusterrolebinding.rbac.authorization.k8s.io/prometheus-dgraph-io created
servicemonitor.monitoring.coreos.com/alpha.dgraph-io created
servicemonitor.monitoring.coreos.com/zero-dgraph-io created
prometheus.monitoring.coreos.com/dgraph-io created
```

To view prometheus UI locally run:

```sh
kubectl port-forward prometheus-dgraph-io-0 9090:9090
```

The UI is accessible at port 9090. Open http://localhost:9090 in your browser to play around.

To register alerts from dgraph cluster with your prometheus deployment follow the steps below:

* Create a kubernetes secret containing alertmanager configuration. Edit the configuration file present [here](https://github.com/dgraph-io/dgraph/blob/master/contrib/config/monitoring/prometheus/alertmanager-config.yaml)
with the required reciever configuration including the slack webhook credential and create the secret.

You can find more information about alertmanager configuration [here](https://prometheus.io/docs/alerting/configuration/).

```sh
$ kubectl create secret generic alertmanager-alertmanager-dgraph-io --from-file=alertmanager.yaml=alertmanager-config.yaml

$ kubectl get secrets
NAME                                            TYPE                 DATA   AGE
alertmanager-alertmanager-dgraph-io             Opaque               1      87m
```

* Apply the [alertmanager](https://github.com/dgraph-io/dgraph/blob/master/contrib/config/monitoring/prometheus/alertmanager.yaml) along with [alert-rules](https://github.com/dgraph-io/dgraph/blob/master/contrib/config/monitoring/prometheus/alert-rules.yaml) manifest
to use the default configured alert configuration. You can also add custom rules based on the metrics exposed by dgraph cluster similar to [alert-rules](https://github.com/dgraph-io/dgraph/blob/master/contrib/config/monitoring/prometheus/alert-rules.yaml)
manifest.

```sh
$ kubectl apply -f alertmanager.yaml
alertmanager.monitoring.coreos.com/alertmanager-dgraph-io created
service/alertmanager-dgraph-io created

$ kubectl apply -f alert-rules.yaml
prometheusrule.monitoring.coreos.com/prometheus-rules-dgraph-io created
```

### Kubernetes Storage

The Kubernetes configurations in the previous sections were configured to run
Dgraph with any storage type (`storage-class: anything`). On the common cloud
environments like AWS, GCP, and Azure, the default storage type are slow disks
like hard disks or low IOPS SSDs. We highly recommend using faster disks for
ideal performance when running Dgraph.

#### Local storage

The AWS storage-optimized i-class instances provide locally attached NVMe-based
SSD storage which provide consistent very high IOPS. The Dgraph team uses
i3.large instances on AWS to test Dgraph.

You can create a Kubernetes `StorageClass` object to provision a specific type
of storage volume which you can then attach to your Dgraph pods. You can set up
your cluster with local SSDs by using [Local Persistent
Volumes](https://kubernetes.io/blog/2018/04/13/local-persistent-volumes-beta/).
This Kubernetes feature is in beta at the time of this writing (Kubernetes
v1.13.1). You can first set up an EC2 instance with locally attached storage.
Once it is formatted and mounted properly, then you can create a StorageClass to
access it.:

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: <your-local-storage-class-name>
provisioner: kubernetes.io/no-provisioner
volumeBindingMode: WaitForFirstConsumer
```

Currently, Kubernetes does not allow automatic provisioning of local storage. So
a PersistentVolume with a specific mount path should be created:

```yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  name: <your-local-pv-name>
spec:
  capacity:
    storage: 475Gi
  volumeMode: Filesystem
  accessModes:
  - ReadWriteOnce
  persistentVolumeReclaimPolicy: Delete
  storageClassName: <your-local-storage-class-name>
  local:
    path: /data
  nodeAffinity:
    required:
      nodeSelectorTerms:
      - matchExpressions:
        - key: kubernetes.io/hostname
          operator: In
          values:
          - <node-name>
```

Then, in the StatefulSet configuration you can claim this local storage in
.spec.volumeClaimTemplate:

```
kind: StatefulSet
...
 volumeClaimTemplates:
  - metadata:
      name: datadir
    spec:
      accessModes:
      - ReadWriteOnce
      storageClassName: <your-local-storage-class-name>
      resources:
        requests:
          storage: 500Gi
```

You can repeat these steps for each instance that's configured with local
node storage.

#### Non-local persistent disks

EBS volumes on AWS and PDs on GCP are persistent disks that can be configured
with Dgraph. The disk performance is much lower than locally attached storage
but can be sufficient for your workload such as testing environments.

When using EBS volumes on AWS, we recommend using Provisioned IOPS SSD EBS
volumes (the io1 disk type) which provide consistent IOPS. The available IOPS
for AWS EBS volumes is based on the total disk size. With Kubernetes, you can
request io1 disks to be provisioned with this config with 50 IOPS/GB using the
`iopsPerGB` parameter:

```
kind: StorageClass
apiVersion: storage.k8s.io/v1
metadata:
  name: <your-storage-class-name>
provisioner: kubernetes.io/aws-ebs
parameters:
  type: io1
  iopsPerGB: "50"
  fsType: ext4
```

Example: Requesting a disk size of 250Gi with this storage class would provide
12.5K IOPS.

### Removing a Dgraph Pod

In the event that you need to completely remove a pod (e.g., its disk got
corrupted and data cannot be recovered), you can use the `/removeNode` API to
remove the node from the cluster. With a Kubernetes StatefulSet, you'll need to
remove the node in this order:

1. On the Zero leader, call `/removeNode` to remove the Dgraph instance from
   the cluster (see [More about Dgraph Zero]({{< relref
   "#more-about-dgraph-zero" >}})). The removed instance will immediately stop
   running. Any further attempts to join the cluster will fail for that instance
   since it has been removed.
2. Remove the PersistentVolumeClaim associated with the pod to delete its data.
   This prepares the pod to join with a clean state.
3. Restart the pod. This will create a new PersistentVolumeClaim to create new
   data directories.

When an Alpha pod restarts in a replicated cluster, it will join as a new member
of the cluster, be assigned a group and an unused index from Zero, and receive
the latest snapshot from the Alpha leader of the group.

When a Zero pod restarts, it must join the existing group with an unused index
ID. The index ID is set with the `--idx` flag. This may require the StatefulSet
configuration to be updated.

### Kubernetes and Bulk Loader

You may want to initialize a new cluster with an existing data set such as data
from the [Dgraph Bulk Loader]({{< relref "#bulk-loader" >}}). You can use [Init
Containers](https://kubernetes.io/docs/concepts/workloads/pods/init-containers/)
to copy the data to the pod volume before the Alpha process runs.

See the `initContainers` configuration in
[dgraph-ha.yaml](https://github.com/dgraph-io/dgraph/blob/master/contrib/config/kubernetes/dgraph-ha/dgraph-ha.yaml)
to learn more.

## More about Dgraph Alpha

On its HTTP port, a Dgraph Alpha exposes a number of admin endpoints.
{{% notice "warning" %}}
These HTTP endpoints are deprecated and will be removed in the next release. Please use the GraphQL endpoint at /admin.
{{% /notice %}}

* `/health?all` returns information about the health of all the servers in the cluster.
* `/admin/shutdown` initiates a proper [shutdown]({{< relref "#shutdown" >}}) of the Alpha.
* `/admin/export` initiates a data [export]({{< relref "#export" >}}). The exported data will be
encrypted if the alpha instance was configured with an encryption key file.

By default the Alpha listens on `localhost` for admin actions (the loopback address only accessible from the same machine). The `--bindall=true` option binds to `0.0.0.0` and thus allows external connections.

{{% notice "tip" %}}Set max file descriptors to a high value like 10000 if you are going to load a lot of data.{{% /notice %}}

### Querying Health

You can query the `/admin` graphql endpoint with a query like the one below to get a JSON consisting of basic information about health of all the servers in the cluster.

```graphql
query {
  health {
    instance
    address
    version
    status
    lastEcho
    group
    uptime
    ongoing
    indexing
  }
}
```

Here’s an example of JSON returned from the above query:

```json
{
  "data": {
    "health": [
      {
        "instance": "zero",
        "address": "localhost:5080",
        "version": "v2.0.0-rc1",
        "status": "healthy",
        "lastEcho": 1582827418,
        "group": "0",
        "uptime": 1504
      },
      {
        "instance": "alpha",
        "address": "localhost:7080",
        "version": "v2.0.0-rc1",
        "status": "healthy",
        "lastEcho": 1582827418,
        "group": "1",
        "uptime": 1505,
        "ongoing": ["opIndexing"],
        "indexing": ["name", "age"]
      }
    ]
  }
}
```

- `instance`: Name of the instance. Either `alpha` or `zero`.
- `status`: Health status of the instance. Either `healthy` or `unhealthy`.
- `version`: Version of Dgraph running the Alpha or Zero server.
- `uptime`: Time in nanoseconds since the Alpha or Zero server is up and running.
- `address`: IP_ADDRESS:PORT of the instance.
- `group`: Group assigned based on the replication factor. Read more [here]({{< relref "/deploy/index.md#cluster-setup" >}}).
- `lastEcho`: Last time, in Unix epoch, when the instance was contacted by another Alpha or Zero server.
- `ongoing`: List of ongoing operations in the background.
- `indexing`: List of predicates for which indexes are built in the background. Read more [here]({{< relref "/query-language/index.md#indexes-in-background" >}}).

The same information (except `ongoing` and `indexing`) is available from the `/health` and `/health?all` endpoints of Alpha server.

## More about Dgraph Zero

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

### More about /state endpoint

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

## Log Format

Dgraph's log format comes from the glog library and is [formatted](https://github.com/golang/glog/blob/23def4e6c14b4da8ac2ed8007337bc5eb5007998/glog.go#L523-L533) as follows:

	`Lmmdd hh:mm:ss.uuuuuu threadid file:line] msg...`

Where the fields are defined as follows:

```
	L                A single character, representing the log level (eg 'I' for INFO)
	mm               The month (zero padded; ie May is '05')
	dd               The day (zero padded)
	hh:mm:ss.uuuuuu  Time in hours, minutes and fractional seconds
	threadid         The space-padded thread ID as returned by GetTID()
	file             The file name
	line             The line number
	msg              The user-supplied message
```

### Query Logging

To enable query logging, you must set `-v=3` which will enable verbose logging for everything. Alternatively, you can set `--vmodule=server=3` for only the dgraph/server.go file which would only enable query/mutation logging.

## TLS configuration

{{% notice "note" %}}
This section refers to the `dgraph cert` command which was introduced in v1.0.9. For previous releases, see the previous [TLS configuration documentation](https://dgraph.io/docs/v1.0.7/deploy/#tls-configuration).
{{% /notice %}}


Connections between client and server can be secured with TLS. Password protected private keys are **not supported**.

{{% notice "tip" %}}If you're generating encrypted private keys with `openssl`, be sure to specify encryption algorithm explicitly (like `-aes256`). This will force `openssl` to include `DEK-Info` header in private key, which is required to decrypt the key by Dgraph. When default encryption is used, `openssl` doesn't write that header and key can't be decrypted.{{% /notice %}}

### Dgraph Certificate Management Tool

The `dgraph cert` program creates and manages CA-signed certificates and private keys using a generated Dgraph Root CA. The `dgraph cert` command simplifies certificate management for you.

```sh
# To see the available flags.
$ dgraph cert --help

# Create Dgraph Root CA, used to sign all other certificates.
$ dgraph cert

# Create node certificate and private key
$ dgraph cert -n localhost

# Create client certificate and private key for mTLS (mutual TLS)
$ dgraph cert -c dgraphuser

# Combine all in one command
$ dgraph cert -n localhost -c dgraphuser

# List all your certificates and keys
$ dgraph cert ls
```

#### File naming conventions

To enable TLS you must specify the directory path to find certificates and keys. The default location where the _cert_ command stores certificates (and keys) is `tls` under the Dgraph working directory; where the data files are found. The default dir path can be overridden using the `--dir` option.

```sh
$ dgraph cert --dir ~/mycerts
```

The following file naming conventions are used by Dgraph for proper TLS setup.

| File name | Description | Use |
|-----------|-------------|-------|
| ca.crt | Dgraph Root CA certificate | Verify all certificates |
| ca.key | Dgraph CA private key | Validate CA certificate |
| node.crt | Dgraph node certificate | Shared by all nodes for accepting TLS connections |
| node.key | Dgraph node private key | Validate node certificate |
| client._name_.crt | Dgraph client certificate | Authenticate a client _name_ |
| client._name_.key | Dgraph client private key | Validate _name_ client certificate |

The Root CA certificate is used for verifying node and client certificates, if changed you must regenerate all certificates.

For client authentication, each client must have their own certificate and key. These are then used to connect to the Dgraph node(s).

The node certificate `node.crt` can support multiple node names using multiple host names and/or IP address. Just separate the names with commas when generating the certificate.

```sh
$ dgraph cert -n localhost,104.25.165.23,dgraph.io,2400:cb00:2048:1::6819:a417
```

{{% notice "tip" %}}You must delete the old node cert and key before you can generate a new pair.{{% /notice %}}

{{% notice "note" %}}When using host names for node certificates, including _localhost_, your clients must connect to the matching host name -- such as _localhost_ not 127.0.0.1. If you need to use IP addresses, then add them to the node certificate.{{% /notice %}}

#### Certificate inspection

The command `dgraph cert ls` lists all certificates and keys in the `--dir` directory (default 'tls'), along with details to inspect and validate cert/key pairs.

Example of command output:

```sh
-rw-r--r-- ca.crt - Dgraph Root CA certificate
        Issuer: Dgraph Labs, Inc.
           S/N: 043c4d8fdd347f06
    Expiration: 02 Apr 29 16:56 UTC
SHA-256 Digest: 4A2B0F0F 716BF5B6 C603E01A 6229D681 0B2AFDC5 CADF5A0D 17D59299 116119E5

-r-------- ca.key - Dgraph Root CA key
SHA-256 Digest: 4A2B0F0F 716BF5B6 C603E01A 6229D681 0B2AFDC5 CADF5A0D 17D59299 116119E5

-rw-r--r-- client.admin.crt - Dgraph client certificate: admin
        Issuer: Dgraph Labs, Inc.
     CA Verify: PASSED
           S/N: 297e4cb4f97c71f9
    Expiration: 03 Apr 24 17:29 UTC
SHA-256 Digest: D23EFB61 DE03C735 EB07B318 DB70D471 D3FE8556 B15D084C 62675857 788DF26C

-rw------- client.admin.key - Dgraph Client key
SHA-256 Digest: D23EFB61 DE03C735 EB07B318 DB70D471 D3FE8556 B15D084C 62675857 788DF26C

-rw-r--r-- node.crt - Dgraph Node certificate
        Issuer: Dgraph Labs, Inc.
     CA Verify: PASSED
           S/N: 795ff0e0146fdb2d
    Expiration: 03 Apr 24 17:00 UTC
         Hosts: 104.25.165.23, 2400:cb00:2048:1::6819:a417, localhost, dgraph.io
SHA-256 Digest: 7E243ED5 3286AE71 B9B4E26C 5B2293DA D3E7F336 1B1AFFA7 885E8767 B1A84D28

-rw------- node.key - Dgraph Node key
SHA-256 Digest: 7E243ED5 3286AE71 B9B4E26C 5B2293DA D3E7F336 1B1AFFA7 885E8767 B1A84D28
```

Important points:

* The cert/key pairs should always have matching SHA-256 digests. Otherwise, the cert(s) must be
  regenerated. If the Root CA pair differ, all cert/key must be regenerated; the flag `--force`
  can help.
* All certificates must pass Dgraph CA verification.
* All key files should have the least access permissions, especially the `ca.key`, but be readable.
* Key files won't be overwritten if they have limited access, even with `--force`.
* Node certificates are only valid for the hosts listed.
* Client certificates are only valid for the named client/user.

### TLS Options

The following configuration options are available for Alpha:

* `--tls_dir string` - TLS dir path; this enables TLS connections (usually 'tls').
* `--tls_use_system_ca` - Include System CA with Dgraph Root CA.
* `--tls_client_auth string` - TLS client authentication used to validate client connection. See [Client Authentication Options](#client-authentication-options) for details.

Dgraph Live Loader can be configured with the following options:

* `--tls_cacert string` - Dgraph Root CA, such as `./tls/ca.crt`
* `--tls_use_system_ca` - Include System CA with Dgraph Root CA.
* `--tls_cert` - User cert file provided by the client to Alpha
* `--tls_key` - User private key file provided by the client to Alpha
* `--tls_server_name string` - Server name, used for validating the server's TLS host name.


#### Using TLS without Client Authentication

For TLS without client authentication, you can configure certificates and run Alpha server using the following:

```sh
# First, create rootca and node certificates and private keys
$ dgraph cert -n localhost
# Default use for enabling TLS server (after generating certificates and private keys)
$ dgraph alpha --tls_dir tls
```

You can then run Dgraph live loader using the following:

```sh
# Now, connect to server using TLS
$ dgraph live --tls_cacert ./tls/ca.crt --tls_server_name "localhost" -s 21million.schema -f 21million.rdf.gz
```

#### Using TLS with Client Authentication

If you do require Client Authentication (Mutual TLS), you can configure certificates and run Alpha server using the following:

```sh
# First, create a rootca, node, and client certificates and private keys
$ dgraph cert -n localhost -c dgraphuser
# Default use for enabling TLS server with client authentication (after generating certificates and private keys)
$ dgraph alpha --tls_dir tls --tls_client_auth="REQUIREANDVERIFY"
```

You can then run Dgraph live loader using the following:

```sh
# Now, connect to server using mTLS (mutual TLS)
$ dgraph live \
   --tls_cacert ./tls/ca.crt \
   --tls_cert ./tls/client.dgraphuser.crt \
   --tls_key ./tls/client.dgraphuser.key \
   --tls_server_name "localhost" \
   -s 21million.schema \
   -f 21million.rdf.gz
```

#### Client Authentication Options

The server will always **request** Client Authentication.  There are four different values for the `--tls_client_auth` option that change the security policy of the client certificate.

| Value              | Client Cert/Key | Client Certificate Verified |
|--------------------|-----------------|--------------------|
| `REQUEST`          | optional        | Client certificate is not VERIFIED if provided. (least secure) |
| `REQUIREANY`       | required        | Client certificate is never VERIFIED |
| `VERIFYIFGIVEN`    | optional        | Client certificate is VERIFIED if provided (default) |
| `REQUIREANDVERIFY` | required        | Client certificate is always VERIFIED (most secure) |

{{% notice "note" %}}REQUIREANDVERIFY is the most secure but also the most difficult to configure for remote clients. When using this value, the value of `--tls_server_name` is matched against the certificate SANs values and the connection host.{{% /notice %}}

### Using Ratel UI with Client authentication

Ratel UI (and any other JavaScript clients built on top of `dgraph-js-http`)
connect to Dgraph servers via HTTP, when TLS is enabled servers begin to expect
HTTPS requests only. Therefore some adjustments need to be made.

If the `--tls_client_auth` option is set to `REQUEST`or `VERIFYIFGIVEN` (default):

1. Change the connection URL from `http://` to `https://` (e.g. `https://127.0.0.1:8080`).
2. Install / make trusted the certificate of the Dgraph certificate authority `ca.crt`. Refer to the documentation of your OS / browser for instructions
(e.g. on Mac OS this means adding `ca.crt` to the KeyChain and making it trusted
for `Secure Socket Layer`).

For `REQUIREANY` and `REQUIREANDVERIFY` you need to follow the steps above and
also need to install client certificate on your OS / browser:

1. Generate a client certificate: `dgraph cert -c MyLaptop`.
2. Convert it to a `.p12` file:
`openssl pkcs12 -export -out MyLaptopCert.p12 -in tls/client.MyLaptop.crt -inkey tls/client.MyLaptop.key`. Use any password you like for export.
3. Install the generated `MyLaptopCert.p12` file on the client system
(on Mac OS this means simply double-click the file in Finder).
4. Next time you use Ratel to connect to an alpha with Client authentication
enabled the browser will prompt you for a client certificate to use. Select the
certificate you've just installed in the step above and queries/mutations will
succeed.

### Using Curl with Client authentication

When TLS is enabled, `curl` requests to Dgraph will need some specific options to work.  For instance (for an export request):

```
curl --silent --cacert ./tls/ca.crt https://localhost:8080/admin/export
```

If you are using `curl` with [Client Authentication](#client-authentication-options) set to `REQUIREANY` or `REQUIREANDVERIFY`, you will need to provide the client certificate and private key.  For instance (for an export request):

```
curl --silent --cacert ./tls/ca.crt --cert ./tls/client.dgraphuser.crt --key ./tls/client.dgraphuser.key https://localhost:8080/admin/export
```

Refer to the `curl` documentation for further information on its TLS options.

### Access Data Using a Client

Some examples of connecting via a [Client](/clients) when TLS is in use can be found below:

- [dgraph4j](https://github.com/dgraph-io/dgraph4j#creating-a-secure-client-using-tls)
- [dgraph-js](https://github.com/dgraph-io/dgraph-js/tree/master/examples/tls)
- [dgo](https://github.com/dgraph-io/dgraph/blob/master/tlstest/acl/acl_over_tls_test.go)
- [pydgraph](https://github.com/dgraph-io/pydgraph/tree/master/examples/tls)

### Troubleshooting Ratel's Client authentication

If you are getting errors in Ratel when server's TLS is enabled try opening
your alpha URL as a webpage.

Assuming you are running Dgraph on your local machine, opening
`https://localhost:8080/` in browser should produce a message `Dgraph browser is available for running separately using the dgraph-ratel binary`.

In case you are getting a connection error, try not passing the
`--tls_client_auth` flag when starting an alpha. If you are still getting an
error, check that your hostname is correct and the port is open; then make sure
that "Dgraph Root CA" certificate is installed and trusted correctly.

After that, if things work without `--tls_client_auth` but stop working when
`REQUIREANY` and `REQUIREANDVERIFY` is set make sure the `.p12` file is
installed correctly.

## Cluster Checklist

In setting up a cluster be sure the check the following.

* Is at least one Dgraph Zero node running?
* Is each Dgraph Alpha instance in the cluster set up correctly?
* Will each Dgraph Alpha instance be accessible to all peers on 7080 (+ any port offset)?
* Does each instance have a unique ID on startup?
* Has `--bindall=true` been set for networked communication?

See the [Production Checklist]({{< relref "#production-checklist" >}}) docs for more info.

## Fast Data Loading

There are two different tools that can be used for fast data loading:

- `dgraph live` runs the Dgraph Live Loader
- `dgraph bulk` runs the Dgraph Bulk Loader

{{% notice "note" %}} Both tools only accept [RDF N-Quad/Triple
data](https://www.w3.org/TR/n-quads/) or JSON in plain or gzipped format. Data
in other formats must be converted.{{% /notice %}}

### Live Loader

Dgraph Live Loader (run with `dgraph live`) is a small helper program which reads RDF N-Quads from a gzipped file, batches them up, creates mutations (using the go client) and shoots off to Dgraph.

Dgraph Live Loader correctly handles assigning unique IDs to blank nodes across multiple files, and can optionally persist them to disk to save memory, in case the loader was re-run.

{{% notice "note" %}} Dgraph Live Loader can optionally write the xid->uid mapping to a directory specified using the `--xidmap` flag, which can reused
given that live loader completed successfully in the previous run.{{% /notice %}}

```sh
$ dgraph live --help # To see the available flags.

# Read RDFs or JSON from the passed file, and send them to Dgraph on localhost:9080.
$ dgraph live -f <path-to-gzipped-RDF-or-JSON-file>

# Read multiple RDFs or JSON from the passed path, and send them to Dgraph on localhost:9080.
$ dgraph live -f <./path-to-gzipped-RDF-or-JSON-files>

# Read multiple files strictly by name.
$ dgraph live -f <file1.rdf, file2.rdf>

# Use compressed gRPC connections to and from Dgraph.
$ dgraph live -C -f <path-to-gzipped-RDF-or-JSON-file>

# Read RDFs and a schema file and send to Dgraph running at given address.
$ dgraph live -f <path-to-gzipped-RDf-or-JSON-file> -s <path-to-schema-file> -a <dgraph-alpha-address:grpc_port> -z <dgraph-zero-address:grpc_port>
```

#### Encrypted imports via Live Loader

A new flag keyfile is added to the Live Loader. This option is required to decrypt the encrypted export data and schema files. Once the export files are decrypted, the Live Loader streams the data to a live Alpha instance.

{{% notice "note" %}}
If the live Alpha instance has encryption turned on, the `p` directory will be encrypted. Otherwise, the `p` directory is unencrypted.
{{% /notice %}}

#### Encrypted RDF/JSON file and schema via Live Loader
`dgraph live -f <path-to-encrypted-gzipped-RDF-or-JSON-file> -s <path-to-encrypted-schema> -keyfile <path-to-keyfile-to-decrypt-files>`

#### Other Live Loader options

`--new_uids` (default: false): Assign new UIDs instead of using the existing
UIDs in data files. This is useful to avoid overriding the data in a DB already
in operation.

`-f, --files`: Location of *.rdf(.gz) or *.json(.gz) file(s) to load. It can
load multiple files in a given path. If the path is a directory, then all files
ending in .rdf, .rdf.gz, .json, and .json.gz will be loaded.

`--format`: Specify file format (rdf or json) instead of getting it from
filenames. This is useful if you need to define a strict format manually.

`-b, --batch` (default: 1000): Number of N-Quads to send as part of a mutation.

`-c, --conc` (default: 10): Number of concurrent requests to make to Dgraph.
Do not confuse with `-C`.

`-C, --use_compression` (default: false): Enable compression for connections to and from the
Alpha server.

`-a, --alpha` (default: `localhost:9080`): Dgraph Alpha gRPC server address to connect for live loading. This can be a comma-separated list of Alphas addresses in the same cluster to distribute the load, e.g.,  `"alpha:grpc_port,alpha2:grpc_port,alpha3:grpc_port"`.

`-x, --xidmap` (default: disabled. Need a path): Store xid to uid mapping to a directory. Dgraph will save all identifiers used in the load for later use in other data ingest operations. The mapping will be saved in the path you provide and you must indicate that same path in the next load. It is recommended to use this flag if you have full control over your identifiers (Blank-nodes). Because the identifier will be mapped to a specific UID.

### Bulk Loader

{{% notice "note" %}}
It's crucial to tune the bulk loader's flags to get good performance. See the
section below for details.
{{% /notice %}}

Dgraph Bulk Loader serves a similar purpose to the Dgraph Live Loader, but can
only be used to load data into a new cluster. It cannot be run on an existing
Dgraph cluster. Dgraph Bulk Loader is **considerably faster** than the Dgraph
Live Loader and is the recommended way to perform the initial import of large
datasets into Dgraph.

Only one or more Dgraph Zeros should be running for bulk loading. Dgraph Alphas
will be started later.

{{% notice "warning" %}}
Don't use bulk loader once the Dgraph cluster is up and running. Use it to import
your existing data to a new cluster.
{{% /notice %}}

You can [read some technical details](https://blog.dgraph.io/post/bulkloader/)
about the bulk loader on the blog.

See [Fast Data Loading]({{< relref "#fast-data-loading" >}}) for more info about
the expected N-Quads format.

**Reduce shards**: Before running the bulk load, you need to decide how many
Alpha groups will be running when the cluster starts. The number of Alpha groups
will be the same number of reduce shards you set with the `--reduce_shards`
flag. For example, if your cluster will run 3 Alpha with 3 replicas per group,
then there is 1 group and `--reduce_shards` should be set to 1. If your cluster
will run 6 Alphas with 3 replicas per group, then there are 2 groups and
`--reduce_shards` should be set to 2.

**Map shards**: The `--map_shards` option must be set to at least what's set for
`--reduce_shards`. A higher number helps the bulk loader evenly distribute
predicates between the reduce shards.

```sh
$ dgraph bulk -f goldendata.rdf.gz -s goldendata.schema --map_shards=4 --reduce_shards=2 --http localhost:8000 --zero=localhost:5080
```
```
{
	"DataFiles": "goldendata.rdf.gz",
	"DataFormat": "",
	"SchemaFile": "goldendata.schema",
	"DgraphsDir": "out",
	"TmpDir": "tmp",
	"NumGoroutines": 4,
	"MapBufSize": 67108864,
	"ExpandEdges": true,
	"SkipMapPhase": false,
	"CleanupTmp": true,
	"NumShufflers": 1,
	"Version": false,
	"StoreXids": false,
	"ZeroAddr": "localhost:5080",
	"HttpAddr": "localhost:8000",
	"IgnoreErrors": false,
	"MapShards": 4,
	"ReduceShards": 2
}
The bulk loader needs to open many files at once. This number depends on the size of the data set loaded, the map file output size, and the level of indexing. 100,000 is adequate for most data set sizes. See `man ulimit` for details of how to change the limit.
Current max open files limit: 1024
MAP 01s rdf_count:176.0 rdf_speed:174.4/sec edge_count:564.0 edge_speed:558.8/sec
MAP 02s rdf_count:399.0 rdf_speed:198.5/sec edge_count:1.291k edge_speed:642.4/sec
MAP 03s rdf_count:666.0 rdf_speed:221.3/sec edge_count:2.164k edge_speed:718.9/sec
MAP 04s rdf_count:952.0 rdf_speed:237.4/sec edge_count:3.014k edge_speed:751.5/sec
MAP 05s rdf_count:1.327k rdf_speed:264.8/sec edge_count:4.243k edge_speed:846.7/sec
MAP 06s rdf_count:1.774k rdf_speed:295.1/sec edge_count:5.720k edge_speed:951.5/sec
MAP 07s rdf_count:2.375k rdf_speed:338.7/sec edge_count:7.607k edge_speed:1.085k/sec
MAP 08s rdf_count:3.697k rdf_speed:461.4/sec edge_count:11.89k edge_speed:1.484k/sec
MAP 09s rdf_count:71.98k rdf_speed:7.987k/sec edge_count:225.4k edge_speed:25.01k/sec
MAP 10s rdf_count:354.8k rdf_speed:35.44k/sec edge_count:1.132M edge_speed:113.1k/sec
MAP 11s rdf_count:610.5k rdf_speed:55.39k/sec edge_count:1.985M edge_speed:180.1k/sec
MAP 12s rdf_count:883.9k rdf_speed:73.52k/sec edge_count:2.907M edge_speed:241.8k/sec
MAP 13s rdf_count:1.108M rdf_speed:85.10k/sec edge_count:3.653M edge_speed:280.5k/sec
MAP 14s rdf_count:1.121M rdf_speed:79.93k/sec edge_count:3.695M edge_speed:263.5k/sec
MAP 15s rdf_count:1.121M rdf_speed:74.61k/sec edge_count:3.695M edge_speed:246.0k/sec
REDUCE 16s [1.69%] edge_count:62.61k edge_speed:62.61k/sec plist_count:29.98k plist_speed:29.98k/sec
REDUCE 17s [18.43%] edge_count:681.2k edge_speed:651.7k/sec plist_count:328.1k plist_speed:313.9k/sec
REDUCE 18s [33.28%] edge_count:1.230M edge_speed:601.1k/sec plist_count:678.9k plist_speed:331.8k/sec
REDUCE 19s [45.70%] edge_count:1.689M edge_speed:554.4k/sec plist_count:905.9k plist_speed:297.4k/sec
REDUCE 20s [60.94%] edge_count:2.252M edge_speed:556.5k/sec plist_count:1.278M plist_speed:315.9k/sec
REDUCE 21s [93.21%] edge_count:3.444M edge_speed:681.5k/sec plist_count:1.555M plist_speed:307.7k/sec
REDUCE 22s [100.00%] edge_count:3.695M edge_speed:610.4k/sec plist_count:1.778M plist_speed:293.8k/sec
REDUCE 22s [100.00%] edge_count:3.695M edge_speed:584.4k/sec plist_count:1.778M plist_speed:281.3k/sec
Total: 22s
```

The output will be generated in the `out` directory by default. Here's the bulk
load output from the example above:

```sh
$ tree ./out
```
```
./out
├── 0
│   └── p
│       ├── 000000.vlog
│       ├── 000002.sst
│       └── MANIFEST
└── 1
    └── p
        ├── 000000.vlog
        ├── 000002.sst
        └── MANIFEST

4 directories, 6 files
```

Because `--reduce_shards` was set to 2, there are two sets of p directories: one
in `./out/0` directory and another in the `./out/1` directory.

Once the output is created, they can be copied to all the servers that will run
Dgraph Alphas. Each Dgraph Alpha must have its own copy of the group's p
directory output. Each replica of the first group should have its own copy of
`./out/0/p`, each replica of the second group should have its own copy of
`./out/1/p`, and so on.

```sh
$ dgraph bulk --help # To see the available flags.

# Read RDFs or JSON from the passed file.
$ dgraph bulk -f <path-to-gzipped-RDF-or-JSON-file> ...

# Read multiple RDFs or JSON from the passed path.
$ dgraph bulk -f <./path-to-gzipped-RDF-or-JSON-files> ...

# Read multiple files strictly by name.
$ dgraph bulk -f <file1.rdf, file2.rdf> ...

```

#### Encryption at rest with Bulk Loader

Even before the Dgraph cluster starts, we can load data using Bulk Loader with the encryption feature turned on. Later we can point the generated `p` directory to a new Alpha server.

Here's an example to run Bulk Loader with a key used to write encrypted data:

```bash
dgraph bulk --encryption_key_file ./enc_key_file -f data.json.gz -s data.schema --map_shards=1 --reduce_shards=1 --http localhost:8000 --zero=localhost:5080
```

#### Encrypting imports via Bulk Loader

The Bulk Loader’s `encryption_key_file` option was previously used to encrypt the output `p ` directory. This same option will also be used to decrypt the encrypted export data and schema files.

Another option, `--encrypted`, indicates whether the input `rdf`/`json` data and schema files are encrypted or not. With this switch, we support the use case of migrating data from unencrypted exports to encrypted import.

So, with the above two options we have 4 cases:

1. `--encrypted=true` and no `encryption_key_file`.

Error: If the input is encrypted, a key file must be provided.

2. `--encrypted=true` and `encryption_key_file`=`path to key.

Input is encrypted and output `p` dir is encrypted as well.

3. `--encrypted=false` and no `encryption_key_file`.

Input is not encrypted and the output `p` dir is also not encrypted.   

4. `--encrypted=false` and `encryption_key_file`=`path to key`.

Input is not encrypted but the output is encrypted. (This is the migration use case mentioned above).

#### Other Bulk Loader options

`--new_uids` (default: false): Assign new UIDs instead of using the existing
UIDs in data files. This is useful to avoid overriding the data in a DB already
in operation.

`-f, --files`: Location of *.rdf(.gz) or *.json(.gz) file(s) to load. It can
load multiple files in a given path. If the path is a directory, then all files
ending in .rdf, .rdf.gz, .json, and .json.gz will be loaded.

`--format`: Specify file format (rdf or json) instead of getting it from
filenames. This is useful if you need to define a strict format manually.

`--store_xids`: Generate a xid edge for each node. It will store the XIDs (The identifier / Blank-nodes) in an attribute named `xid` in the entity itself. It is useful if you gonna use [External IDs](/mutations#external-ids).

`--xidmap` (default: disabled. Need a path): Store xid to uid mapping to a directory. Dgraph will save all identifiers used in the load for later use in other data ingest operations. The mapping will be saved in the path you provide and you must indicate that same path in the next load. It is recommended to use this flag if you have full control over your identifiers (Blank-nodes). Because the identifier will be mapped to a specific UID.

#### Tuning & monitoring

##### Performance Tuning

{{% notice "tip" %}}
We highly recommend [disabling swap
space](https://askubuntu.com/questions/214805/how-do-i-disable-swap) when
running Bulk Loader. It is better to fix the parameters to decrease memory
usage, than to have swapping grind the loader down to a halt.
{{% /notice %}}

Flags can be used to control the behaviour and performance characteristics of
the bulk loader. You can see the full list by running `dgraph bulk --help`. In
particular, **the flags should be tuned so that the bulk loader doesn't use more
memory than is available as RAM**. If it starts swapping, it will become
incredibly slow.

**In the map phase**, tweaking the following flags can reduce memory usage:

- The `--num_go_routines` flag controls the number of worker threads. Lowering reduces memory
  consumption.

- The `--mapoutput_mb` flag controls the size of the map output files. Lowering
  reduces memory consumption.

For bigger datasets and machines with many cores, gzip decoding can be a
bottleneck during the map phase. Performance improvements can be obtained by
first splitting the RDFs up into many `.rdf.gz` files (e.g. 256MB each). This
has a negligible impact on memory usage.

**The reduce phase** is less memory heavy than the map phase, although can still
use a lot.  Some flags may be increased to improve performance, *but only if
you have large amounts of RAM*:

- The `--reduce_shards` flag controls the number of resultant Dgraph alpha instances.
  Increasing this increases memory consumption, but in exchange allows for
higher CPU utilization.

- The `--map_shards` flag controls the number of separate map output shards.
  Increasing this increases memory consumption but balances the resultant
Dgraph alpha instances more evenly.

- The `--shufflers` controls the level of parallelism in the shuffle/reduce
  stage. Increasing this increases memory consumption.

## Ludicrous Mode

Ludicrous mode is available in Dgraph v20.03.1 or later.

Ludicrous mode allows Dgraph to ingest data at an incredibly fast speed. It differs from the normal mode as it provides fewer guarantees. In normal mode, Dgraph provides strong consistency. In ludicrous mode, Dgraph provides eventual consistency. In ludicrous mode, any mutation which succeeds **might be available eventually**. **Eventually** means the changes will be applied later and might not be reflected in queries away. If Dgraph crashes unexpectedly, there **might** be unapplied mutations which **will not** be picked up when Dgraph restarts. Dgraph with ludicrous mode enabled behaves as an eventually consistent system.


**How do I enable it?**

Ludicrous mode can be enabled by setting the `--ludicrous_mode` config option to all Zero and Alpha instances in the cluster.


**What does it do?**

It doesn't wait for mutations to be applied. When a mutation comes, it proposes the mutation to the cluster and as soon as the proposal reaches the other nodes, it returns the response right away. You don't need to send a commit request for mutation. It's equivalent to having CommitNow set automatically for all mutations. All the mutations are then sent to background workers which keep applying them.

Also, Dgraph does not sync writes to disk. This increases throughput but may result in loss of unsynced writes in the event of hardware failure.


**What is the trade off?**

As mentioned in the section above, it provides amazing speed at the cost of some guarantees.

It can be used when we have write-heavy operations and there is a time gap between queries and mutations, or you are fine with potentially reading stale data.

There are no transactions in ludicrous mode. That is, you cannot open a transaction, apply a mutation, and then decide to cancel the transaction. Every mutation request is committed to Dgraph.

**Can the cluster run with HA?**

Yes, ludicrous mode works with the cluster set up in a highly-available (HA) configuration.

**Can the cluster run with multiple data shards?**

Yes, ludicrous mode works with the cluster set up with multiple data shards.



## Monitoring
Dgraph exposes metrics via the `/debug/vars` endpoint in json format and the `/debug/prometheus_metrics` endpoint in Prometheus's text-based format. Dgraph doesn't store the metrics and only exposes the value of the metrics at that instant. You can either poll this endpoint to get the data in your monitoring systems or install **[Prometheus](https://prometheus.io/docs/introduction/install/)**. Replace targets in the below config file with the ip of your Dgraph instances and run prometheus using the command `prometheus -config.file my_config.yaml`.

```sh
scrape_configs:
  - job_name: "dgraph"
    metrics_path: "/debug/prometheus_metrics"
    scrape_interval: "2s"
    static_configs:
    - targets:
      - 172.31.9.133:6080     # For Dgraph zero, 6080 is the http endpoint exposing metrics.
      - 172.31.15.230:8080    # For Dgraph alpha, 8080 is the http endpoint exposing metrics.
      - 172.31.0.170:8080
      - 172.31.8.118:8080
```

{{% notice "note" %}}
Raw data exported by Prometheus is available via `/debug/prometheus_metrics` endpoint on Dgraph alphas.
{{% /notice %}}

Install **[Grafana](http://docs.grafana.org/installation/)** to plot the metrics. Grafana runs at port 3000 in default settings. Create a prometheus datasource by following these **[steps](https://prometheus.io/docs/visualization/grafana/#creating-a-prometheus-data-source)**. Import **[grafana_dashboard.json](https://github.com/dgraph-io/benchmarks/blob/master/scripts/grafana_dashboard.json)** by following this **[link](http://docs.grafana.org/reference/export_import/#importing-a-dashboard)**.


### CloudWatch

Route53's health checks can be leveraged to create standard CloudWatch alarms to notify on change in the status of the `/health` endpoints of Alpha and Zero.

Considering that the endpoints to monitor are publicly accessible and you have the AWS credentials and [awscli](https://aws.amazon.com/cli/) setup, we’ll go through an example of setting up a simple CloudWatch alarm configured to alert via email for the Alpha endpoint `alpha.acme.org:8080/health`. Dgraph Zero's `/health` endpoint can also be monitored in a similar way.



#### Create the Route53 Health Check
```sh
aws route53 create-health-check \
    --caller-reference $(date "+%Y%m%d%H%M%S") \
    --health-check-config file:///tmp/create-healthcheck.json \
    --query 'HealthCheck.Id'
```
The file `/tmp/create-healthcheck.json` would need to have the values for the parameters required to create the health check as such:
```sh
{
  "Type": "HTTPS",
  "ResourcePath": "/health",
  "FullyQualifiedDomainName": "alpha.acme.org",
  "Port": 8080,
  "RequestInterval": 30,
  "FailureThreshold": 3
}
```
The reference for the values one can specify while creating or updating a health check can be found on the AWS [documentation](https://docs.aws.amazon.com/Route53/latest/DeveloperGuide/health-checks-creating-values.html).

The response to the above command would be the ID of the created health check.
```sh
"29bdeaaa-f5b5-417e-a5ce-7dba1k5f131b"
```
Make a note of the health check ID. This will be used to integrate CloudWatch alarms with the health check.

{{% notice "note" %}}
Currently, Route53 metrics are only (available)[https://docs.aws.amazon.com/Route53/latest/DeveloperGuide/monitoring-health-checks.html] in the **US East (N. Virginia)** region. The Cloudwatch Alarm (and the SNS Topic) should therefore be created in `us-east-1`.
{{% /notice %}}

#### [Optional] Creating an SNS Topic
SNS topics are used to create message delivery channels. If you do not have any SNS topics configured, one can be created by running the following command:

```sh
aws sns create-topic --region=us-east-1 --name ops --query 'TopicArn'
```

The response to the above command would be as follows:
```sh
"arn:aws:sns:us-east-1:123456789012:ops"
```
Be sure to make a note of the topic ARN. This would be used to configure the CloudWatch alarm's action parameter.

Run the following command to subscribe your email to the SNS topic:
```sh
aws sns subscribe \
    --topic-arn arn:aws:sns:us-east-1:123456789012:ops \
    --protocol email \
    --notification-endpoint ops@acme.org
```
The subscription will need to be confirmed through *AWS Notification - Subscription Confirmation* sent through email. Once the subscription is confirmed, CloudWatch can be configured to use the SNS topic to trigger the alarm notification.



#### Creating a CloudWatch Alarm
The following command creates a CloudWatch alarm with `--alarm-actions` set to the ARN of the SNS topic and the `--dimensions` of the alarm set to the health check ID.
```sh
aws cloudwatch put-metric-alarm \
    --region=us-east-1 \
    --alarm-name dgraph-alpha \
    --alarm-description "Alarm for when Alpha is down" \
    --metric-name HealthCheckStatus \
    --dimensions "Name=HealthCheckId,Value=29bdeaaa-f5b5-417e-a5ce-7dba1k5f131b" \
    --namespace AWS/Route53 \
    --statistic Minimum \
    --period 60 \
    --threshold 1 \
    --comparison-operator LessThanThreshold \
    --evaluation-periods 1 \
    --treat-missing-data breaching \
    --alarm-actions arn:aws:sns:us-east-1:123456789012:ops
```

One can verify the alarm status from the CloudWatch or Route53 consoles.

##### Internal Endpoints
If the Alpha endpoint is internal to the VPC network - one would need to create a Lambda function that would periodically (triggered using CloudWatch Event Rules) request the `/health` path and create CloudWatch metrics which could then be used to create the required CloudWatch alarms.
The architecture and the CloudFormation template to achieve the same can be found [here](https://aws.amazon.com/blogs/networking-and-content-delivery/performing-route-53-health-checks-on-private-resources-in-a-vpc-with-aws-lambda-and-amazon-cloudwatch/).



## Metrics

Dgraph metrics follow the [metric and label conventions for
Prometheus](https://prometheus.io/docs/practices/naming/).

### Disk Metrics

The disk metrics let you track the disk activity of the Dgraph process. Dgraph does not interact
directly with the filesystem. Instead it relies on [Badger](https://github.com/dgraph-io/badger) to
read from and write to disk.

 Metrics                          	 | Description
 -------                          	 | -----------
 `badger_v2_disk_reads_total`        | Total count of disk reads in Badger.
 `badger_v2_disk_writes_total`       | Total count of disk writes in Badger.
 `badger_v2_gets_total`              | Total count of calls to Badger's `get`.
 `badger_v2_memtable_gets_total`     | Total count of memtable accesses to Badger's `get`.
 `badger_v2_puts_total`              | Total count of calls to Badger's `put`.
 `badger_v2_read_bytes`              | Total bytes read from Badger.
 `badger_v2_written_bytes`           | Total bytes written to Badger.

### Memory Metrics

The memory metrics let you track the memory usage of the Dgraph process. The idle and inuse metrics
gives you a better sense of the active memory usage of the Dgraph process. The process memory metric
shows the memory usage as measured by the operating system.

By looking at all three metrics you can see how much memory a Dgraph process is holding from the
operating system and how much is actively in use.

 Metrics                          | Description
 -------                          | -----------
 `dgraph_memory_idle_bytes`       | Estimated amount of memory that is being held idle that could be reclaimed by the OS.
 `dgraph_memory_inuse_bytes`      | Total memory usage in bytes (sum of heap usage and stack usage).
 `dgraph_memory_proc_bytes`       | Total memory usage in bytes of the Dgraph process. On Linux/macOS, this metric is equivalent to resident set size. On Windows, this metric is equivalent to [Go's runtime.ReadMemStats](https://golang.org/pkg/runtime/#ReadMemStats).

### Activity Metrics

The activity metrics let you track the mutations, queries, and proposals of an Dgraph instance.

 Metrics                                            | Description
 -------                                            | -----------
 `go_goroutines`                                    | Total number of Goroutines currently running in Dgraph.
 `dgraph_active_mutations_total`                    | Total number of mutations currently running.
 `dgraph_pending_proposals_total`                   | Total pending Raft proposals.
 `dgraph_pending_queries_total`                     | Total number of queries in progress.
 `dgraph_num_queries_total{method="Server.Mutate"}` | Total number of mutations run in Dgraph.
 `dgraph_num_queries_total{method="Server.Query"}`  | Total number of queries run in Dgraph.

### Health Metrics

The health metrics let you track to check the availability of an Dgraph Alpha instance.

 Metrics                          | Description
 -------                          | -----------
 `dgraph_alpha_health_status`     | **Only applicable to Dgraph Alpha**. Value is 1 when the Alpha is ready to accept requests; otherwise 0.

### Go Metrics

Go's built-in metrics may also be useful to measure for memory usage and garbage collection time.

 Metrics                        | Description
 -------                        | -----------
 `go_memstats_gc_cpu_fraction`  | The fraction of this program's available CPU time used by the GC since the program started.
 `go_memstats_heap_idle_bytes`  | Number of heap bytes waiting to be used.
 `go_memstats_heap_inuse_bytes` | Number of heap bytes that are in use.

## Tracing

Dgraph is integrated with [OpenCensus](https://opencensus.io/zpages/) to collect distributed traces from the Dgraph cluster.

Trace data is always collected within Dgraph. You can adjust the trace sampling rate for Dgraph queries with the `--trace` option for Dgraph Alphas. By default, `--trace` is set to 1 to trace 100% of queries.

### Examining Traces with zPages

The most basic way to view traces is with the integrated trace pages.

OpenCensus's [zPages](https://opencensus.io/zpages/) are accessible via the Zero or Alpha HTTP port at `/z/tracez`.

### Examining Traces with Jaeger

Jaeger collects distributed traces and provides a UI to view and query traces across different services. This provides the necessary observability to figure out what is happening in the system.

Dgraph can be configured to send traces directly to a Jaeger collector with the `--jaeger.collector` flag. For example, if the Jaeger collector is running on `http://localhost:14268`, then pass the flag to the Dgraph Zero and Dgraph Alpha instances as `--jaeger.collector=http://localhost:14268`.

See [Jaeger's Getting Started docs](https://www.jaegertracing.io/docs/getting-started/) to get up and running with Jaeger.

#### Setting up multiple Dgraph clusters with Jaeger

Jaeger allows you to examine traces from multiple Dgraph clusters. To do this, use the `--collector.tags` on a Jaeger collector to set custom trace tags. For example, run one collector with `--collector.tags env=qa` and then another collector with `--collector.tags env=dev`. In Dgraph, set the `--jaeger.collector` flag in the Dgraph QA cluster to the first collector and the flag in the Dgraph Dev cluster to the second collector.
You can run multiple Jaeger collector components for the same single Jaeger backend (e.g., many Jaeger collectors to a single Cassandra backend). This is still a single Jaeger installation but with different collectors customizing the tags per environment.

Once you have this configured, you can filter by tags in the Jaeger UI. Filter traces by tags matching `env=dev`:

{{% load-img "/images/jaeger-ui.png" "Jaeger UI" %}}

Every trace has your custom tags set under the “Process” section of each span:

{{% load-img "/images/jaeger-server-query.png" "Jaeger Query" %}}

Filter traces by tags matching `env=qa`:

{{% load-img "/images/jaeger-json.png" "Jaeger JSON" %}}

{{% load-img "/images/jaeger-server-query-2.png" "Jaeger Query Result" %}}

For more information, check out [Jaeger's Deployment Guide](https://www.jaegertracing.io/docs/deployment/).

## Data compression on disk

Alpha exposes the option `--badger.compression_level` to configure the compression
level for data on disk using Zstd compression. The option can be set as

```sh
dgraph alpha --badger.compression_level=xxx
```

A higher compression level is more CPU intensive but offers a better compression
ratio. The default level is 3.

This option is available in v20.03.1 and later.

## Dgraph Administration

Each Dgraph Alpha exposes administrative operations over HTTP to export data and to perform a clean shutdown.

### Whitelisting Admin Operations

By default, admin operations can only be initiated from the machine on which the Dgraph Alpha runs.
You can use the `--whitelist` option to specify whitelisted IP addresses and ranges for hosts from which admin operations can be initiated.

```sh
dgraph alpha --whitelist 172.17.0.0:172.20.0.0,192.168.1.1 --lru_mb <one-third RAM> ...
```
This would allow admin operations from hosts with IP between `172.17.0.0` and `172.20.0.0` along with
the server which has IP address as `192.168.1.1`.

### Restricting Mutation Operations

By default, you can perform mutation operations for any predicate.
If the predicate in mutation doesn't exist in the schema,
the predicate gets added to the schema with an appropriate
[Dgraph Type]({{< relref "query-language/index.md#schema-types" >}}).

You can use `--mutations disallow` to disable all mutations,
which is set to `allow` by default.

```sh
dgraph alpha --mutations disallow
```

Enforce a strict schema by setting `--mutations strict`.
This mode allows mutations only on predicates already in the schema.
Before performing a mutation on a predicate that doesn't exist in the schema,
you need to perform an alter operation with that predicate and its schema type.

```sh
dgraph alpha --mutations strict
```

### Securing Alter Operations

Clients can use alter operations to apply schema updates and drop particular or all predicates from the database.
By default, all clients are allowed to perform alter operations.
You can configure Dgraph to only allow alter operations when the client provides a specific token.
This can be used to prevent clients from making unintended or accidental schema updates or predicate drops.

You can specify the auth token with the `--auth_token` option for each Dgraph Alpha in the cluster.
Clients must include the same auth token to make alter requests.

```sh
$ dgraph alpha --lru_mb=2048 --auth_token=<authtokenstring>
```

```sh
$ curl -s localhost:8080/alter -d '{ "drop_all": true }'
# Permission denied. No token provided.
```

```sh
$ curl -s -H 'X-Dgraph-AuthToken: <wrongsecret>' localhost:8180/alter -d '{ "drop_all": true }'
# Permission denied. Incorrect token.
```

```sh
$ curl -H 'X-Dgraph-AuthToken: <authtokenstring>' localhost:8180/alter -d '{ "drop_all": true }'
# Success. Token matches.
```

{{% notice "note" %}}
To fully secure alter operations in the cluster, the auth token must be set for every Alpha.
{{% /notice %}}


### Exporting Database

An export of all nodes is started by locally executing the following GraphQL mutation on /admin endpoint using any compatible client like Insomnia, GraphQL Playground or GraphiQL.

```graphql
mutation {
  export(input: {format: "rdf"}) {
    response {
      message
      code
    }
  }
}
```
{{% notice "warning" %}}By default, this won't work if called from outside the server where the Dgraph Alpha is running.
You can specify a list or range of whitelisted IP addresses from which export or other admin operations
can be initiated using the `--whitelist` flag on `dgraph alpha`.
{{% /notice %}}

This triggers an export for all Alpha groups of the cluster. The data is exported from the following Dgraph instances:

1. For the Alpha instance that receives the GET request, the group's export data is stored with this Alpha.
2. For every other group, its group's export data is stored with the Alpha leader of that group.

It is up to the user to retrieve the right export files from the Alphas in the
cluster. Dgraph does not copy all files to the Alpha that initiated the export.
The user must also ensure that there is sufficient space on disk to store the
export.

Each Alpha leader for a group writes output as a gzipped file to the export
directory specified via the `--export` flag (defaults to a directory called `"export"`). If any of the groups fail, the
entire export process is considered failed and an error is returned.

The data is exported in RDF format by default. A different output format may be specified with the
`format` field. For example:

```graphql
mutation {
  export(input: {format: "json"}) {
    response {
      message
      code
    }
  }
}
```

Currently, "rdf" and "json" are the only formats supported.

#### Encrypting Exports

Export is available wherever an Alpha is running. To encrypt an export, the Alpha must be configured with the `encryption-key-file`.

{{% notice "note" %}}
The `encryption-key-file` was used for `encryption-at-rest` and will now also be used for encrypted backups and exports.
{{% /notice %}}

### Shutting Down Database

A clean exit of a single Dgraph node is initiated by running the following GraphQL mutation on /admin endpoint.
{{% notice "warning" %}}This won't work if called from outside the server where Dgraph is running.
You can specify a list or range of whitelisted IP addresses from which shutdown or other admin operations
can be initiated using the `--whitelist` flag on `dgraph alpha`.
{{% /notice %}}

```graphql
mutation {
  shutdown {
    response {
      message
      code
    }
  }
}
```

This stops the Alpha on which the command is executed and not the entire cluster.

### Deleting database

Individual triples, patterns of triples and predicates can be deleted as described in the [query languge docs](/query-language#delete).

To drop all data, you could send a `DropAll` request via `/alter` endpoint.

Alternatively, you could:

* [Shutdown Dgraph]({{< relref "#shutting-down-database" >}}) and wait for all writes to complete,
* Delete (maybe do an export first) the `p` and `w` directories, then
* Restart Dgraph.

### Upgrading Database

Doing periodic exports is always a good idea. This is particularly useful if you wish to upgrade Dgraph or reconfigure the sharding of a cluster. The following are the right steps to safely export and restart.

1. Start an [export]({{< relref "#exporting-database">}})
2. Ensure it is successful
3. [Shutdown Dgraph]({{< relref "#shutting-down-database" >}}) and wait for all writes to complete
4. Start a new Dgraph cluster using new data directories (this can be done by passing empty directories to the options `-p` and `-w` for Alphas and `-w` for Zeros)
5. Reload the data via [bulk loader]({{< relref "#bulk-loader" >}})
6. Verify the correctness of the new Dgraph cluster. If all looks good, you can delete the old directories (export serves as an insurance)

These steps are necessary because Dgraph's underlying data format could have changed, and reloading the export avoids encoding incompatibilities.

Blue-green deployment is a common approach to minimize downtime during the upgrade process.
This approach involves switching your application to read-only mode. To make sure that no mutations are executed during the maintenance window you can
do a rolling restart of all your Alpha using the option `--mutations disallow` when you restart the Alphas. This will ensure the cluster is in read-only mode.

At this point your application can still read from the old cluster and you can perform the steps 4. and 5. described above.
When the new cluster (that uses the upgraded version of Dgraph) is up and running, you can point your application to it, and shutdown the old cluster.

#### Upgrading from v1.2.2 to v20.03.0 for Enterprise Customers

1. Use [binary]({{< relref "enterprise-features/index.md#binary-backups">}}) backup to export data from old cluster
2. Ensure it is successful
3. [Shutdown Dgraph]({{< relref "#shutting-down-database" >}}) and wait for all writes to complete
4. Upgrade `dgraph` binary to `v20.03.0`
5. [Restore]({{< relref "enterprise-features/index.md#restore-from-backup">}}) from the backups using upgraded `dgraph` binary
6. Start a new Dgraph cluster using the restored data directories
7. Upgrade ACL data using the following command:

```
dgraph upgrade --acl -a localhost:9080 -u groot -p password
```

{{% notice "note" %}}
If you are upgrading from v1.0, please make sure you follow the schema migration steps described in [this section](/howto/#schema-types-scalar-uid-and-list-uid).
{{% /notice %}}

### Post Installation

Now that Dgraph is up and running, to understand how to add and query data to Dgraph, follow [Query Language Spec](/query-language). Also, have a look at [Frequently asked questions](/faq).

## Troubleshooting

Here are some problems that you may encounter and some solutions to try.

#### Running OOM (out of memory)

During bulk loading of data, Dgraph can consume more memory than usual, due to high volume of writes. That's generally when you see the OOM crashes.

The recommended minimum RAM to run on desktops and laptops is 16GB. Dgraph can take up to 7-8 GB with the default setting `--lru_mb` set to 4096; so having the rest 8GB for desktop applications should keep your machine humming along.

On EC2/GCE instances, the recommended minimum is 8GB. It's recommended to set `--lru_mb` to one-third of RAM size.

You could also decrease memory usage of Dgraph by setting `--badger.vlog=disk`.

#### Too many open files

If you see an log error messages saying `too many open files`, you should increase the per-process file descriptors limit.

During normal operations, Dgraph must be able to open many files. Your operating system may set by default a open file descriptor limit lower than what's needed for a database such as Dgraph.

On Linux and Mac, you can check the file descriptor limit with `ulimit -n -H` for the hard limit and `ulimit -n -S` for the soft limit. The soft limit should be set high enough for Dgraph to run properly. A soft limit of 65535 is a good lower bound for a production setup. You can adjust the limit as needed.

## Production Checklist

This guide describes important setup recommendations for a production-ready Dgraph cluster.

{{% notice "note" %}}
In this guide, a node refers to a Dgraph instance unless specified otherwise.
{{% /notice %}}

A **Dgraph cluster** is comprised of multiple **Dgraph instances** (aka **nodes**) connected together to form a single distributed database. A Dgraph instance is either a **Dgraph Zero** or **Dgraph Alpha**, each of which serves a different role in the cluster.

There can be one or more **Dgraph clients** connected to Dgraph to perform database operations (queries, mutations, alter schema, etc.). These clients connect via gRPC or HTTP. Dgraph provides official clients for Go, Java, Python, and JavaScript, and C#. All of these are gRPC-based, and JavaScript supports both gRPC and HTTP for browser support. Community-developed Dgraph clients for other languages are also available. The full list of clients can be found in [Clients]({{< relref "clients/index.md" >}}) page. You can also interact with Dgraph via curl over HTTP. Dgraph Ratel is a UI client used to visualize queries, run mutations, and manage the schema. Clients do not participate as a member of the database cluster, they simply connect to one or more Dgraph Alpha instances.

#### Cluster Requirements

A minimum of one Dgraph Zero and one Dgraph Alpha is needed for a working cluster.

There can be multiple Dgraph Zeros and Dgraph Alphas running in a single cluster.


#### Machine Requirements

To ensure predictable performance characteristics, Dgraph instances should NOT run on "burstable" or throttled machines that limit resources. That includes t2 class machines on AWS.

We recommend each Dgraph instance to be deployed to a single dedicated machine to ensure that Dgraph can take full advantage of machine resources. That is, for a 6-node Dgraph cluster with 3 Dgraph Zeros and 3 Dgraph Alphas, each process runs in its own machine (e.g., EC2 instance). In the event of a machine failure, only one instance is affected, instead of multiple if they were running on that same machine.

If you'd like to run Dgraph with fewer machines, then the recommended configuration is to run a single Dgraph Zero and a single Dgraph Alpha per machine. In a high availability setup, that allows the cluster to lose a single machine (simultaneously losing a Dgraph Zero and a Dgraph Alpha) with continued availability of the database.

Do not run multiple Dgraph Zeros or Dgraph Alpha processes on a single machine. This can affect performance due to shared resources and reduce high availability in the event of machine failures.

#### Operating System

The recommended operating system is Linux for production workloads. Dgraph provides release binaries in Linux, macOS, and Windows. For test environments, you can choose to run Dgraph on any operating system. This deployment guide assumes you are setting up Dgraph on Linux systems (see [Operating System Tuning]({{< relref
   "#operating-system-tuning" >}})).

#### CPU and Memory

At the bare minimum, we recommend machines with at least 2 CPUs and 2 GiB of memory for testing.

You'll want a sufficient CPU and memory according to your production workload. A common setup for Dgraph is 16 CPUs and 32 GiB of memory per machine. Dgraph is designed with concurrency in mind, so more cores means quicker processing and higher throughput of requests.

You may find you'll need more CPU cores and memory for your specific use case.

#### Disk

Dgraph instances make heavy usage of disks, so storage with high IOPS is highly recommended to ensure reliable performance. Specifically, we recommend SSDs, not HDDs.

Instances such as c5d.4xlarge have locally-attached NVMe SSDs with high IOPS. You can also use EBS volumes with provisioned IOPS (io1). If you are not running performance-critical workloads, you can also choose to use cheaper gp2 EBS volumes.

Recommended disk sizes for Dgraph Zero and Dgraph Alpha:

* Dgraph Zero: 200 GB to 300 GB. Dgraph Zero stores cluster metadata information and maintains a write-ahead log for cluster operations.
* Dgraph Alpha: 250 GB to 750 GB. Dgraph Alpha stores database data, including the schema, indices, and the data values. It maintains a write-ahead log of changes to the database. Your cloud provider may provide better disk performance based on the volume size.

Additional recommendations:

* The recommended Linux filesystem is ext4.
* Avoid using shared storage such as NFS, CIFS, and CEPH storage.

#### Firewall Rules

Dgraph instances communicate over several ports. Firewall rules should be configured appropriately for the ports documented in [Ports Usage]({{< relref "deploy/index.md#ports-usage" >}}).

Internal ports must be accessible by all Zero and Alpha peers for proper cluster-internal communication. Database clients must be able to connect to Dgraph Alpha external ports either directly or through a load balancer.

Dgraph Zeros can be set up in a private network where communication is only with Dgraph Alphas, database administrators, internal services (such as Prometheus or Jaeger), and possibly developers (see note below). Dgraph Zero's 6080 external port is only necessary for database administrators. For example, it can be used to inspect the cluster metadata (/state), allocate UIDs or set txn timestamps (/assign), move data shards (/moveTablet), or remove cluster nodes (/removeNode). The full docs about Zero's administrative tasks are in [More About Dgraph Zero]({{< relref "deploy/index.md#more-about-dgraph-zero" >}}).

{{% notice "note" %}}
Developers using Dgraph Live Loader or Dgraph Bulk Loader require access to both Dgraph Zero port 5080 and Dgraph Alpha port 9080. When using those tools, consider using them within your environment that has network access to both ports of the cluster.
{{% /notice %}}

#### Operating System Tuning

The OS should be configured with the recommended settings to ensure that Dgraph runs properly.

**File Descriptors Limit**

Dgraph can use a large number of open file descriptors. Most operating systems set a default limit that is lower than what is required.

It is recommended to set the file descriptors limit to unlimited. If that is not possible, set it to at least a million (1,048,576) which is recommended to account for cluster growth over time.

#### Deployment

A Dgraph instance is run as a single process from a single static binary. It does not require any additional dependencies or separate services in order to run (see the [Supplementary Services]({{< relref "#supplementary-services" >}}) section for third-party services that work alongside Dgraph). A Dgraph cluster is set up by running multiple Dgraph processes networked together.

#### Terminology

An **N-node cluster** is a Dgraph cluster that contains N number of Dgraph instances. For example, a 6-node cluster means six Dgraph instances. The **replication setting** specifies the number of Dgraph Alpha replicas are assigned per group. The replication setting is a configuration flag (`--replicas`) on Dgraph Zero. A **Dgraph Alpha group** is a set of Dgraph Alphas that store replications of the data amongst each other. Every Dgraph Alpha group is assigned a set of distinct predicates to store and serve.

Examples of different cluster settings:
* No sharding
  * 2-node cluster: 1 Zero, 1 Alpha (one group).
  * HA equivalent: x3 = 6-node cluster.
* With 2-way sharding:
  * 3-node cluster: 1 Zero, 2 Alphas (two groups).
  * HA equivalent: x3 = 9-node cluster.

In the following examples we outline the two most common cluster configurations: a 2-node cluster and a 6-node cluster.

#### Basic setup: 2-node cluster
We provide sample configs for both [Docker Compose](https://github.com/dgraph-io/dgraph/blob/master/contrib/config/docker/docker-compose.yml) and [Kubernetes](https://github.com/dgraph-io/dgraph/tree/master/contrib/config/kubernetes/dgraph-single) for a 2-node cluster. You can also run Dgraph directly on your host machines.

{{% load-img "/images/deploy-guide-1.png" "2-node cluster" %}}

Configuration can be set either as command-line flags, environment variables, or in a config file (see [Config]({{< relref "deploy/index.md#config" >}})).

Dgraph Zero:
* The `--my` flag should be set to the address:port (the internal-gRPC port) that will be accessible to the Dgraph Alpha (default: `localhost:5080`).
* The `--idx` flag should be set to a unique Raft ID within the Dgraph Zero group (default: `1`).
* The `--wal` flag should be set to the directory path to store write-ahead-log entries on disk (default: `zw`).
* The `--bindall` flag should be set to true for machine-to-machine communication (default: `true`).
* Recommended: For better issue diagnostics, set the log level verbosity to 2 with the option `--v=2`.

Dgraph Alpha:
* The `--my` flag should be set to the address:port (the internal-gRPC port) that will be accessible to the Dgraph Zero (default: `localhost:7080`).
* The `--zero` flag should be set to the corresponding Zero address set for Dgraph Zero's `--my` flag.
* The `--postings` flag should be set to the directory path for data storage (default: `p`).
* The `--wal` flag should be set to the directory path for write-ahead-log entries (default: `w`)
* The `--bindall` flag should be set to true for machine-to-machine communication (default: `true`).
* Recommended: For better issue diagnostics, set the log level verbosity to 2 `--v=2`.

#### HA setup: 6-node cluster

We provide sample configs for both [Docker Compose](https://github.com/dgraph-io/dgraph/blob/master/contrib/config/docker/docker-compose-ha.yml) and [Kubernetes](https://github.com/dgraph-io/dgraph/tree/master/contrib/config/kubernetes/dgraph-ha) for a 6-node cluster with 3 Alpha replicas per group. You can also run Dgraph directly on your host machines.

A Dgraph cluster can be configured in a high-availability setup with Dgraph Zero and Dgraph Alpha each set up with peers. These peers are part of Raft consensus groups, which elect a single leader amongst themselves. The non-leader peers are called followers. In the event that the peers cannot communicate with the leader (e.g., a network partition or a machine shuts down), the group automatically elects a new leader to continue.

Configuration can be set either as command-line flags, environment variables, or in a config file (see [Config]({{< relref "deploy/index.md#config" >}})).

In this setup, we assume the following hostnames are set:

- `zero1`
- `zero2`
- `zero3`
- `alpha1`
- `alpha2`
- `alpha3`

We will configure the cluster with 3 Alpha replicas per group. The cluster group-membership topology will look like the following:

{{% load-img "/images/deploy-guide-2.png" "Dgraph cluster image" %}}

##### Set up Dgraph Zero group

In the Dgraph Zero group you must set unique Raft IDs (`--idx`) per Dgraph Zero. Dgraph will not auto-assign Raft IDs to Dgraph Zero instances.

The first Dgraph Zero that starts will initiate the database cluster. Any following Dgraph Zero instances must connect to the cluster via the `--peer` flag to join. If the `--peer` flag is omitted from the peers, then the Dgraph Zero will create its own independent Dgraph cluster.

**First Dgraph Zero** example: `dgraph zero --replicas=3 --idx=1 --my=zero1:5080`

The `--my` flag must be set to the address:port of this instance that peers will connect to. The `--idx` flag sets its Raft ID to `1`.

**Second Dgraph Zero** example: `dgraph zero --replicas=3 --idx=2 --my=zero2:5080 --peer=zero1:5080`

The `--my` flag must be set to the address:port of this instance that peers will connect to. The `--idx` flag sets its Raft ID to 2, and the `--peer` flag specifies a request to connect to the Dgraph cluster of zero1 instead of initializing a new one.

**Third Dgraph Zero** example: `dgraph zero --replicas=3 --idx=3 --my=zero3:5080 --peer=zero1:5080`:

The `--my` flag must be set to the address:port of this instance that peers will connect to. The `--idx` flag sets its Raft ID to 3, and the `--peer` flag specifies a request to connect to the Dgraph cluster of zero1 instead of initializing a new one.

Dgraph Zero configuration options:

* The `--my` flag should be set to the address:port (the internal-gRPC port) that will be accessible to Dgraph Alpha (default: `localhost:5080`).
* The `--idx` flag should be set to a unique Raft ID within the Dgraph Zero group (default: `1`).
* The `--wal` flag should be set to the directory path to store write-ahead-log entries on disk (default: `zw`).
* The `--bindall` flag should be set to true for machine-to-machine communication (default: `true`).
* Recommended: For more informative log info, set the log level verbosity to 2 with the option `--v=2`.

##### Set up Dgraph Alpha group

The number of replica members per Alpha group depends on the setting of Dgraph Zero's `--replicas` flag. Above, it is set to 3. So when Dgraph Alphas join the cluster, Dgraph Zero will assign it to an Alpha group to fill in its members up to the limit per group set by the `--replicas` flag.

First Alpha example: `dgraph alpha --my=alpha1:7080 --zero=zero1:7080`

Second Alpha example: `dgraph alpha --my=alpha2:7080 --zero=zero1:7080`

First Alpha example: `dgraph alpha --my=alpha3:7080 --zero=zero1:7080`

Dgraph Alpha configuration options:

* The `--my` flag should be set to the address:port (the internal-gRPC port) that will be accessible to the Dgraph Zero (default: `localhost:7080`).
* The `--zero` flag should be set to the corresponding Zero address set for Dgraph Zero's `--my`flag.
* The `--postings` flag should be set to the directory path for data storage (default: `p`).
* The `--wal` flag should be set to the directory path for write-ahead-log entries (default: `w`)
* The `--bindall` flag should be set to true for machine-to-machine communication (default: `true`).
* Recommended: For more informative log info, set the log level verbosity to 2 `--v=2`.

#### Supplementary Services

These services are not required for a Dgraph cluster to function but are recommended for better insight when operating a Dgraph cluster.

- [Metrics and monitoring][] with Prometheus and Grafana.
- [Distributed tracing][] with Jaeger.

[Metrics and monitoring]: {{< relref "deploy/index.md#monitoring" >}}
[Distributed tracing]: {{< relref "deploy/index.md#tracing" >}}

## See Also

* [Product Roadmap 2020](https://github.com/dgraph-io/dgraph/issues/4724)
