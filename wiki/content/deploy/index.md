+++
date = "2017-03-20T22:25:17+11:00"
title = "Deploy"
+++

This page talks about running Dgraph in a distributed fashion. This involves
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

### Nightly

Nightly builds from Dgraph master branch at https://github.com/dgraph-io/dgraph are available from https://nightly.dgraph.io.  To install run:

```sh
curl https://nightly.dgraph.io -sSf | bash
```

The Docker version is available as _master_.  Pull and run with:

```sh
docker pull dgraph/dgraph:master
```

## Simple cluster setup

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
smallest unit of data is one predicate.  To shard the graph, one or many
predicates are assigned to a group.  Each node in the cluster serves a single
group. Dgraph zero assigns a group to each node.

**Shard rebalancing**

Dgraph zero tries to rebalance the cluster based on the disk usage in each
group. If Zero detects an imbalance, it would try to move a predicate along
with index and reverse edges to a group that has minimum disk usage. This can
make the predicate unavailable temporarily.

Zero would continuously try to keep the amount of data on each server even,
typically running this check on a 10-min frequency.  Thus, each additional
Dgraph server instance would allow Zero to further split the predicates from
groups and move them to the new node.

**Consistent Replication**

If `--replicas` flag is set to something greater than one, Zero would assign the
same group to multiple nodes. These nodes would then form a Raft group aka
quorum. Every write would be consistently replicated to the quorum. To achieve
consensus, its important that the size of quorum be an odd number. Therefore, we
recommend setting `--replicas` to 1, 3 or 5 (not 2 or 4). This allows 0, 1, or 2
nodes serving the same group to be down, respectively without affecting the
overall health of that group.

### Run directly on host

**Run dgraph zero**

```sh
dgraph zero --my=IPADDR:7080 --wal zw
```

The default Zero ports are 7080 for internal (Grpc) communication to other Dgraph
servers, and 8080 for HTTP communication with external processes. You can change
these ports by using `--port_offset` flag.

The `--my` flag is the connection that Dgraph servers would dial to talk to
zero. So, the port `7080` and the IP address must be visible to all the Dgraph servers.

For all the various flags available, run `dgraph zero --help`.

**Run dgraph server**

```sh
dgraph server --idx=2 --memory_mb=<typically half the RAM>
--my=IPADDR:7081 --zero=localhost:7080 --port_offset=1
```

**Dgraph server listens on port 7080 for internal, port 8080 for external HTTP and
port 9080 for external Grpc communication by default.** You can change that via
`--port_offset` flag.

Note that the `--idx` flag can be ommitted. Zero would then automatically assign
a unique ID to each Dgraph server, which Dgraph server persists in the write ahead log (wal) directory.

You can use `-p` and `-w` to change the storage location of data and WAL. For
all the various flags available, run `dgraph server --help`.

**Run dgraph UI**

```sh
dgraph-ratel -p 8081
```

Dgraph UI listens on port 8081 by default. You can use the -p flag to change it. By default ratel assumes that dgraph is running on localhost:8080, you can change this address from the ui itself.

### Run using Docker

First, you'd want to figure out the host IP address. You can typically do that
via

```sh
ip addr  # On Arch Linux
ifconfig # On Ubuntu/Mac
```
We'll refer to the host IP address via `HOSTIPADDR`.

**Run dgraph zero**

```sh
mkdir ~/data # Or any other directory where data should be stored.

docker run -it -p 7080:7080 -p 8080:8080 -v ~/data:/dgraph dgraph/dgraph:latest dgraph zero --bindall=true --my=HOSTIPADDR:7080
```

**Run dgraph server**
```sh
mkdir ~/data # Or any other directory where data should be stored.

docker run -it -p 7081:7081 -p 8081:8081 -p 9081:9081 -v ~/data:/dgraph dgraph/dgraph:latest dgraph server --port_offset 1 --memory_mb=<typically half the RAM> --zero=HOSTIPADDR:7080 --my=HOSTIPADDR:7081 --idx <unique-id(integer)>
```

**Run dgraph UI**
```sh
docker run -it -p 8082:8082 dgraph/dgraph:latest dgraph-ratel -p 8082
```


{{% notice "note" %}}
You can also use the `:master` tag when running docker image to get the nightly
build. Though, nightly isn't as thoroughly tested as a release, and we
recommend not using it.
{{% /notice %}}

### High-availability setup

In a high-availability setup, we need to run 3 or 5 replicas for Zero, and
similarly, 3 or 5 replicas for the server.
{{% notice "note" %}}
If number of replicas is 2K + 1, up to **K servers** can be down without any
impact on reads or writes.

Avoid keeping replicas to 2K (even number). If K servers go down, this would
block reads and writes, due to lack of consensus.
{{% /notice %}}

**Dgraph Zero**
Run three Zero instances, assigning a unique ID(Integer) to each via `--idx` flag, and
passing the address of any healthy Zero instance via `--peer` flag.

To run three replicas for server, set `--replicas=3`. Every time a new Dgraph
server is added, Zero would check the existing groups and assign them to one,
which doesn't have three replicas.

**Dgraph Server**
Run as many Dgraph servers as you want. You can manually set `--idx` flag, or
you can leave that flag empty, and Zero would auto-assign an id to the server.
This id would get persisted in the write-ahead log, so be careful not to delete
it.

The new servers will automatically detect each other by communicating with
dgraphzero and establish connections to each other.

Typically, Zero would first attempt to replicate a group, by assigning a new
Dgraph server to run the same group as assigned to another. Once the group has
been replicated as per the `--replicas` flag, Zero would create a new group.

Over time, the data would be evenly split across all the groups. So, it's
important to ensure that the number of Dgraph servers is a multiple of the
replication setting. For e.g., if you set `--replicas=3` in Zero, then run three
Dgraph servers for no sharding, but 3x replication. Run six Dgraph servers, for
sharding the data into two groups, with 3x replication.

**Removing Dead Nodes**
If a replica goes down and can't be recovered, you can remove it and add a new node to the quorum. `/removeNode` endpoint on Zero can be used to remove the dead node (`/removeNode?id=3&group=2`).
{{% notice "note" %}}
Before using the api ensure that the node is down and ensure that it doesn't come back up ever again.

Remember to specify the `idx` flag while adding a new replica or else Zero might assign it a different group.
{{% /notice %}}

## AWS using Docker

We will use [Docker Machine](https://docs.docker.com/machine/overview/). It is a tool that lets you install Docker Engine on virtual machines
and easily deploy applications.

* [Install Docker Machine](https://docs.docker.com/machine/install-machine/) on your machine.

### Single server using Docker Compose

{{% notice "note" %}}These instructions are for running Dgraph Server without TLS config.
Instructions for running with TLS would be added soon.{{% /notice %}}

Here we'll go through an example of deploying Dgraph Server and Zero on an AWS instance.

* Make sure you have Docker Machine installed from the [step above]({{< relref "#deployment-on-aws">}}), provisioning an instance on AWS is just one step
   away. You'll have to [configure your AWS credentials](http://docs.aws.amazon.com/sdk-for-java/v1/developer-guide/setup-credentials.html) for programatic access to the Amazon API.

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
(allowing inbound access on 2376 and 22). You can either edit the security group to allow inbound
access to `8080`, `9080` (default ports for Dgraph server) or you can provide your own security
group which allows inbound access on port 22, 2376 (required by Docker Machine), 8080 and 9080.

[Here](https://docs.docker.com/machine/drivers/aws/#options) is a list of full options for the `amazonec2` driver which allows you choose the
instance type, security group, AMI among many other
things.

{{% notice "tip" %}}Docker machine supports [other drivers](https://docs.docker.com/machine/drivers/gce/) like GCE, Azure etc.{{% /notice %}}

* Install and run Dgraph using docker-compose

Docker Compose is a tool for running multi-container Docker applications. You can follow the
instructions [here](https://docs.docker.com/compose/install/) to install it.

Copy the file below in a directory on your machine and name it `docker-compose.yml`.

```sh
version: "3"
services:
  zero:
    image: dgraph/dgraph:latest
    volumes:
      - /data:/dgraph
    restart: on-failure
    command: dgraph zero --port_offset -2000 --my=zero:5080
  server:
    image: dgraph/dgraph:latest
    volumes:
      - /data:/dgraph
    ports:
      - 8080:8080
      - 9080:9080
    restart: on-failure
    command: dgraph server --my=server:7080 --memory_mb=2048 --zero=zero:5080
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

Finally run the command below to start the Server and Zero.
```
docker-compose up -d
```
This would start 2 Docker containers, one running Dgraph Server and the other Zero on the same
machine. Docker would restart the containers in case there is any error.

You can look at the logs using `docker-compose logs`.

### Cluster setup using Docker Swarm

{{% notice "note" %}}These instructions are for running Dgraph Server without TLS config.
Instructions for running with TLS would be added soon.{{% /notice %}}

Here we'll go through an example of deploying 3 Dgraph Server nodes and 1 Zero on three different AWS instances using Docker Swarm with a replication factor of 3.

* Make sure you have Docker Machine installed from the [step above]({{< relref "#deployment-on-aws">}}).

```sh
docker-machine --version
```

* Create 3 instances on AWS and [install Docker Engine](https://docs.docker.com/engine/installation/) on them. This can be done manually or by using `docker-machine`.
You'll have to [configure your AWS credentials](http://docs.aws.amazon.com/sdk-for-java/v1/developer-guide/setup-credentials.html) to create the instances using Docker Machine.

Considering that you have AWS credentials setup, you can use the below commands to start 3 AWS
`t2-micro` instances with Docker Engine installed on them.

```sh
docker-machine create --driver amazonec2 aws01
docker-machine create --driver amazonec2 aws02
docker-machine create --driver amazonec2 aws03
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
(allowing inbound access on 2376 and 22).

You would need to edit the `docker-machine` security group to open inbound traffic on the following ports.

1. Allow all inbound traffic on all ports with Source being `docker-machine` security ports so that
   docker related communication can happen easily.

2. Also open inbound TCP traffic on the following ports required by Dgraph: `8080`, `8081`, `8082`, `9080`, `9081` and `9082`.

[Here](https://docs.docker.com/machine/drivers/aws/#options) is a list of full options for the `amazonec2` driver which allows you choose the
instance type, security group, AMI among many other
things.

{{% notice "tip" %}}Docker machine supports [other drivers](https://docs.docker.com/machine/drivers/gce/) like GCE, Azure etc.{{% /notice %}}

Running `docker-machine ps` shows all the AWS EC2 instances that we started.
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

Now that we have the internal IP, lets initiate the Swarm.
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

Similary, aws03
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
```sh
ID                            HOSTNAME            STATUS              AVAILABILITY        MANAGER STATUS
ghzapjsto20c6d6l3n0m91zev     aws02               Ready               Active
rb39d5lgv66it1yi4rto0gn6a     aws03               Ready               Active
waqdyimp8llvca9i09k4202x5 *   aws01               Ready               Active              Leader
```

* Start the Dgraph cluster

Copy the following file on your host machine and name it as `docker-compose.yml`

```sh
version: "3"
networks:
  dgraph:
services:
  zero:
    image: dgraph/dgraph:latest
    volumes:
      - data-volume:/dgraph
    networks:
      - dgraph
    deploy:
      placement:
        constraints:
          - node.hostname == aws01
    command: dgraph zero --port_offset -2000 --my=zero:5080 --replicas 3
  server_1:
    image: dgraph/dgraph:latest
    hostname: "server_1"
    volumes:
      - data-volume:/dgraph
    ports:
      - 8080:8080
      - 9080:9080
    networks:
      - dgraph
    deploy:
      placement:
        constraints:
          - node.hostname == aws01
    command: dgraph server --my=server_1:7080 --memory_mb=2048 --zero=zero:5080
  server_2:
    image: dgraph/dgraph:latest
    hostname: "server_2"
    volumes:
      - data-volume:/dgraph
    ports:
      - 8081:8081
      - 9081:9081
    networks:
      - dgraph
    deploy:
      placement:
        constraints:
          - node.hostname == aws02
    command: dgraph server --my=server_2:7081 --memory_mb=2048 --zero=zero:5080 -o 1
  server_3:
    image: dgraph/dgraph:latest
    hostname: "server_3"
    volumes:
      - data-volume:/dgraph
    ports:
      - 8082:8082
      - 9082:9082
    networks:
      - dgraph
    deploy:
      placement:
        constraints:
          - node.hostname == aws03
    command: dgraph server --my=server_3:7082 --memory_mb=2048 --zero=zero:5080 -o 2
volumes:
  data-volume:
```
Run the following command on the Swarm leader to deploy the Dgraph Cluster.

```sh
eval $(docker-machine env aws01)
docker stack deploy -c docker-compose.yml dgraph
```

This should run three dgraph server services(one on each VM because of the constraint we have) and one dgraph zero service on aws01.
These contraints are important so that in case of restart Dgraph Server or Zero starts with the same volumes.

{{% notice "note" %}} This setup would create and use a local volume called `dgraph_data-volume` on the instances. If you plan to replace instances, you should use remote storage like [cloudstor](https://docs.docker.com/docker-for-aws/persistent-data-volumes) instead of local disk. {{% /notice %}}



You can verify that all services were created successfully by running:

```sh
docker service ls
```

Output:
```
ID                  NAME                MODE                REPLICAS            IMAGE                PORTS
69oge03y0koz        dgraph_server_2     replicated          1/1                 dgraph/dgraph:latest   *:8081->8081/tcp,*:9081->9081/tcp
kq5yks92mnk6        dgraph_server_3     replicated          1/1                 dgraph/dgraph:latest   *:8082->8082/tcp,*:9082->9082/tcp
uild5cqp44dz        dgraph_zero         replicated          1/1                 dgraph/dgraph:latest   *:6080->6080/tcp
v9jlw00iz2gg        dgraph_server_1     replicated          1/1                 dgraph/dgraph:latest   *:8080->8080/tcp,*:9080->9080/tcp
```


{{% notice "note" %}} You can access the UI at `http://<external-ip-address-node>:8080`. It is also available on port `8081` and `8082`.  {{% /notice %}}

To stop the cluster run

```
docker stack rm dgraph
```

### HA Cluster setup (optional)

Here is a sample swarm config for running 6 Dgraph Server nodes and 3 Zero nodes on 6 different
instances. Setup should be similar to [Cluster setup using Docker Swarm]({{< relref "#cluster-setup-using-docker-swarm" >}}) apart from a couple of differences. This setup would ensure replication with sharding of data.

1. The file assumes that there are six hosts available as docker-machines.
2. You have to open ports `8083`, `8084`, `8085`, `9083`, `9084` and `9085` as well.

```sh
version: "3"
networks:
  dgraph:
services:
  zero_1:
    image: dgraph/dgraph:latest
    volumes:
      - data-volume:/dgraph
    networks:
      - dgraph
    deploy:
      placement:
        constraints:
          - node.hostname == aws01
    command: dgraph zero -o -2000 --my=zero_1:5080 --replicas 3 --idx 1
  zero_2:
    image: dgraph/dgraph:latest
    volumes:
      - data-volume:/dgraph
    networks:
      - dgraph
    deploy:
      placement:
        constraints:
          - node.hostname == aws02
    command: dgraph zero -o -1999 --my=zero_2:5081 --replicas 3 --peer zero_1:5080 --idx 2
  zero_3:
    image: dgraph/dgraph:latest
    volumes:
      - data-volume:/dgraph
    networks:
      - dgraph
    deploy:
      placement:
        constraints:
          - node.hostname == aws03
    command: dgraph zero -o -1998 --my=zero_3:5082 --replicas 3 --peer zero_1:5080 --idx 3
  server_1:
    image: dgraph/dgraph:latest
    hostname: "server_1"
    volumes:
      - data-volume:/dgraph
    ports:
      - 8080:8080
      - 9080:9080
    networks:
      - dgraph
    deploy:
      replicas: 1
      placement:
        constraints:
          - node.hostname == aws01
    command: dgraph server --my=server_1:7080 --memory_mb=2048 --zero=zero_1:5080
  server_2:
    image: dgraph/dgraph:latest
    hostname: "server_2"
    volumes:
      - data-volume:/dgraph
    ports:
      - 8081:8081
      - 9081:9081
    networks:
      - dgraph
    deploy:
      replicas: 1
      placement:
        constraints:
          - node.hostname == aws02
    command: dgraph server --my=server_2:7081 --memory_mb=2048 --zero=zero_1:5080 -o 1
  server_3:
    image: dgraph/dgraph:latest
    hostname: "server_3"
    volumes:
      - data-volume:/dgraph
    ports:
      - 8082:8082
      - 9082:9082
    networks:
      - dgraph
    deploy:
      replicas: 1
      placement:
        constraints:
          - node.hostname == aws03
    command: dgraph server --my=server_3:7082 --memory_mb=2048 --zero=zero_1:5080 -o 2
  server_4:
    image: dgraph/dgraph:latest
    hostname: "server_4"
    volumes:
      - data-volume:/dgraph
    ports:
      - 8083:8083
      - 9083:9083
    networks:
      - dgraph
    deploy:
      placement:
        constraints:
          - node.hostname == aws04
    command: dgraph server --my=server_4:7083 --memory_mb=2048 --zero=zero_1:5080 -o 3
  server_5:
    image: dgraph/dgraph:latest
    hostname: "server_5"
    volumes:
      - data-volume:/dgraph
    ports:
      - 8084:8084
      - 9084:9084
    networks:
      - dgraph
    deploy:
      placement:
        constraints:
          - node.hostname == aws05
    command: dgraph server --my=server_5:7084 --memory_mb=2048 --zero=zero_1:5080 -o 4
  server_6:
    image: dgraph/dgraph:latest
    hostname: "server_6"
    volumes:
      - data-volume:/dgraph
    ports:
      - 8085:8085
      - 9085:9085
    networks:
      - dgraph
    deploy:
      placement:
        constraints:
          - node.hostname == aws06
    command: dgraph server --my=server_6:7085 --memory_mb=2048 --zero=zero_1:5080 -o 5

volumes:
  data-volume:
```


## Kubernetes (v1.8.4)

{{% notice "note" %}}These instructions are for running Dgraph Server without TLS config.
Instructions for running with TLS would be added soon.{{% /notice %}}

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

Once your kubernetes cluster is up, you can use [dgraph-single.yaml](https://github.com/dgraph-io/dgraph/blob/master/contrib/config/kubernetes/dgraph-single.yaml) to start a Dgraph Server and Zero.

* From your machine, run the following command to start a StatefulSet that creates a Pod with Dgraph
  Server and Zero running in it.

```sh
kubectl create -f https://raw.githubusercontent.com/dgraph-io/dgraph/master/contrib/config/kubernetes/dgraph-single.yaml
```

Output:
```
service "dgraph-public" created
service "dgraph" created
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

{{% notice "tip" %}}You can check the logs for the containers in the pod using `kubectl logs -f dgraph-0 <container_name>`. For example, try `kubectl logs -f dgraph-0 server` for server logs.{{% /notice %}}

* Test the setup

Port forward from your local machine to the pod

```sh
kubectl port-forward dgraph-0 8080
kubectl port-forward dgraph-0 8081
```

Go to `http://localhost:8081` and verify Dgraph is working as expected.

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


### Cluster setup

In this setup, we are going to deploy 1 Zero node and 3 Server nodes. We start Zero with `--replicas
3` flag, so all data would be replicated on each of the 3 Server nodes.

{{% notice "note" %}} Ideally you should have atleast three worker nodes as part of your Kubernetes
cluster so that each Dgraph Server runs on a separate node.{{% /notice %}}

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

Once your kubernetes cluster is up, you can use [dgraph-multi.yaml](https://github.com/dgraph-io/dgraph/blob/master/contrib/config/kubernetes/dgraph-multi.yaml) to start the cluster.

* From your machine, run the following command to start the cluster.

```sh
kubectl create -f https://raw.githubusercontent.com/dgraph-io/dgraph/master/contrib/config/kubernetes/dgraph-multi.yaml
```

Output:
```
service "dgraph-public" created
service "dgraph-zero" created
service "dgraph-server" created
statefulset "dgraph-zero" created
statefulset "dgraph-server" created
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
dgraph-server-0        1/1       Running   0          2m
dgraph-server-1        1/1       Running   0          2m
dgraph-server-2        1/1       Running   0          1m
dgraph-zero-0          1/1       Running   0          2m

```

{{% notice "tip" %}}You can check the logs for the containers in the pod using `kubectl logs -f dgraph-server-0` and `kubectl logs -f dgraph-zero-0`.{{% /notice %}}

* Test the setup

Port forward from your local machine to the pod

```sh
kubectl port-forward dgraph-server-0 8080
kubectl port-forward dgraph-ratel-<pod-id>
```

Go to `http://localhost:8081` and verify Dgraph is working as expected.

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

### HA Cluster setup

This setup allows you to run 3 Dgraph Servers and 3 Zero Servers. The instructions are same as
[cluster-setup]({{< relref "#cluster-setup">}}) apart from the step to start the cluster. We start Zero with `--replicas
3` flag, so all data would be replicated on each of 3 Server nodes.

{{% notice "note" %}} Ideally you should have atleast three worker nodes as part of your Kubernetes
cluster so that each Dgraph Server runs on a separate node.{{% /notice %}}

Once your kubernetes cluster is up, you can use [dgraph-ha.yaml](https://github.com/dgraph-io/dgraph/blob/master/contrib/config/kubernetes/dgraph-ha.yaml) to start the cluster.

* From your machine, run the following command to start the cluster.

```sh
kubectl create -f https://raw.githubusercontent.com/dgraph-io/dgraph/master/contrib/config/kubernetes/dgraph-ha.yaml
```

Output:
```sh
service "dgraph-public" created
service "dgraph" created
statefulset "dgraph" created
deployment "dgraph-ratel" created
```

After this you can follow other steps from [cluster-setup]({{< relref "#cluster-setup">}}) to verify
that your setup is working as expected.


## Building from Source

Make sure you have [Go](https://golang.org/dl/) (version >= 1.8) installed.

After installing Go, run
```sh
# This should install dgraph binary in your $GOPATH/bin.

go get -u -v github.com/dgraph-io/dgraph/dgraph
```

If you get errors related to `grpc` while building them, your
`go-grpc` version might be outdated. We don't vendor in `go-grpc`(because it
causes issues while using the Go client). Update your `go-grpc` by running.
```sh
go get -u -v google.golang.org/grpc
```

## More about Dgraph

On its http port, a running Dgraph instance exposes a number of admin endpoints.

* `/` Browser UI and query visualization.
* `/health` HTTP status code 200 and "OK" message if worker is running, HTTP 503 otherwise.
* `/admin/shutdown` [shutdown]({{< relref "#shutdown">}}) a node.
* `/admin/export` take a running [export]({{< relref "#export">}}).

By default the server listens on `localhost` (the loopback address only accessible from the same machine).  The `--bindall=true` option binds to `0.0.0.0` and thus allows external connections.

{{% notice "tip" %}}Set max file descriptors to a high value like 10000 if you are going to load a lot of data.{{% /notice %}}

## More about Dgraph Zero

Dgraph Zero controls the Dgraph cluster. It automatically moves data between
different dgraph instances based on the size of the data served by each instance.

It is mandatory to run atleast one `dgraph zero` node before running any `dgraph server`.
Options present for `dgraph zero` can be seen by running `dgraph zero --help`.

* Zero stores information about the cluster.
* `--replicas` is the option that controls the replication factor. (i.e. number of replicas per data shard, including the original shard)
* Whenever a new machine is brought up it is assigned a group based on replication factor. If replication factor is 1 then each node will serve different group. If replication factor is 2 and you launch 4 machines then first two machines would server group 1 and next two machines would server group 2.
* Zero also monitors the space occupied by predicates in each group and moves them around to rebalance the cluster.

Like Dgraph, Zero also exposes HTTP on 8080 (+ any `--port_offset`). You can query it
to see useful information, like the following:

* `/state` Information about the nodes that are part of the cluster. Also contains information about
  size of predicates and groups they belong to.
* `/removeNode?id=idx&group=gid` Used to remove dead node from the quorum, takes node id and group id as query param.

## Config

The full set of dgraph's configuration options (along with brief descriptions)
can be viewed by invoking dgraph with the `--help` flag. For example, so see
the options for `dgraph server`, run `dgraph server --help`.

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
(`SERVER`, `ZERO`, `LIVE`, or `BULK`), and then the name of the flag (in
uppercase). For example, instead of using `dgraph server --memory_mb=8096`, you
could use `DGRAPH_SERVER_MEMORY_MB=8096 dgraph server`.

Configuration file formats supported are JSON, TOML, YAML, HCL, and Java
properties (detected via file extension).

A configuration file can be specified using the `--config` flag, or an
environment variable. E.g. `dgraph zero --config my_config.json` or
`DGRAPH_ZERO_CONFIG=my_config.json dgraph zero`.

The config file structure is just simple key/value pairs (mirroring the flag
names). E.g. a JSON config file that sets `--idx`, `--peer`, and `--replicas`:

```json
{
  "idx": 42,
  "peer": 192.168.0.55:9080,
  "replicas": 2
}
```

## TLS configuration
Connections between client and server can be secured with TLS.
Both encrypted (password protected) and unencrypted private keys are supported.

{{% notice "tip" %}}If you're generating encrypted private keys with `openssl`, be sure to specify encryption algorithm explicitly (like `-aes256`). This will force `openssl` to include `DEK-Info` header in private key, which is required to decrypt the key by Dgraph. When default encryption is used, `openssl` doesn't write that header and key can't be decrypted.{{% /notice %}}

Following configuration options are available for the server:

```sh
# Use TLS connections with clients.
tls_on

# CA Certs file path.
tls_ca_certs string

# Include System CA into CA Certs.
tls_use_system_ca

# Certificate file path.
tls_cert string

# Certificate key file path.
tls_cert_key string

# Certificate key passphrase.
tls_cert_key_passphrase string

# Enable TLS client authentication
tls_client_auth string

# TLS max version. (default "TLS12")
tls_max_version string

# TLS min version. (default "TLS11")
tls_min_version string
```

Dgraph loader can be configured with following options:

```sh
# Use TLS connections.
tls_on

# CA Certs file path.
tls_ca_certs string

# Include System CA into CA Certs.
tls_use_system_ca

# Certificate file path.
tls_cert string

# Certificate key file path.
tls_cert_key string

# Certificate key passphrase.
tls_cert_key_passphrase string

# Server name.
tls_server_name string

# Skip certificate validation (insecure)
tls_insecure

# TLS max version. (default "TLS12")
tls_max_version string

# TLS min version. (default "TLS11")
tls_min_version string
```


## Cluster Checklist

In setting up a cluster be sure the check the following.

* Is atleast one dgraphzero node running?
* Is each dgraph instance in the cluster [set up correctly]({{< relref "#running-dgraph">}})?
* Will each instance be accessible to all peers on 7080 (+ any port offset)?
* Does each node have a unique ID on startup?
* Has `--bindall=true` been set for networked communication?

## Fast Data Loading

There are two different tools that can be used for bulk data loading:

- `dgraph live`
- `dgraph bulk`

{{% notice "note" %}} Both tools only accepts gzipped, RDF NQuad/Triple data.
Data in other formats must be converted [to
this](https://www.w3.org/TR/n-quads/).{{% /notice %}}

### Live Loader

The `dgraph live` binary is a small helper program which reads RDF NQuads from a gzipped file, batches them up, creates mutations (using the go client) and shoots off to Dgraph.

Live loader correctly handles assigning unique IDs to blank nodes across multiple files, and persists them to disk to save memory and in case the loader was re-run.


{{% notice "note" %}} Live loader writes the xid->uid mapping in the `x` directory, which can reused
given that live loader completed successfully in the previous run.{{% /notice %}}

```sh
$ dgraph live --help # To see the available flags.

# Read RDFs from the passed file, and send them to Dgraph on localhost:9080.
$ dgraph live -r <path-to-rdf-gzipped-file>

# Read RDFs and a schema file and send to Dgraph running at given address
$ dgraph live -r <path-to-rdf-gzipped-file> -s <path-to-schema-file> -d <dgraph-server-address:port> -z <dgraph-zero-address:grpc_port>
```

### Bulk Loader

{{% notice "note" %}}
It's crucial to tune the bulk loaders flags to get good performance. See the
section below for details.
{{% /notice %}}

Bulk loader serves a similar purpose to the live loader, but can only be used
while dgraph is offline for the initial population. It cannot run on an
existing dgraph instance.

{{% notice "warning" %}}
Don't use bulk loader once Dgraph is up and running. Use it to import your
existing data into a new instance of Dgraph server.
{{% /notice %}}

Bulk loader is **considerably faster** than the live loader, and is the recommended
way to perform the initial import of large datasets into dgraph.

You can [read some technical details](https://blog.dgraph.io/post/bulkloader/)
about the bulk loader on the blog.

See [Fast Data Loading]({{< relref "#fast-data-loading" >}}) for more about the expected N-Quads format.

You need to determine the
number of dgraph instances you want in your cluster. You should set the number
of reduce shards to this number. You will also need to set the number of map
shards to at least this number (a higher number helps the bulk loader evenly
distribute predicates between the reduce shards). For this example, you could use
2 reduce shards and 4 map shards.

{{% notice "note" %}}
Ports in the example below may have to be adjusted depending on how other
processes have been set up.
{{% /notice %}}

```sh
$ dgraph bulk -r goldendata.rdf.gz -s goldendata.schema --map_shards=4 --reduce_shards=2 --http localhost:8000 --zero_addr=localhost:7080
{
	"RDFDir": "goldendata.rdf.gz",
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
	"ZeroAddr": "localhost:7080",
	"HttpAddr": "localhost:8000",
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

Once the data is generated, you can start the Dgraph servers by pointing their
`-p` directory to the output. If running multiple Dgraph servers, you'd need to
copy over the output shards into different servers.

```sh
$ cd out/i # i = shard number.
$ dgraph server -zero=localhost:7080 -memory_mb=1024
```

#### Performance Tuning

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

- The `--reduce_shards` flag controls the number of resultant dgraph instances.
  Increasing this increases memory consumption, but in exchange allows for
higher CPU utilization.

- The `--map_shards` flag controls the number of separate map output shards.
  Increasing this increases memory consumption but balances the resultant
dgraph instances more evenly.

- The `--shufflers` controls the level of parallelism in the shuffle/reduce
  stage. Increasing this increases memory consumption.

## Export

An export of all nodes is started by locally accessing the export endpoint of any server in the cluster.

```sh
$ curl localhost:8080/admin/export
```
{{% notice "warning" %}}This won't work if called from outside the server where dgraph is running.
{{% /notice %}}

This also works from a browser, provided the HTTP GET is being run from the same server where the Dgraph instance is running.

This triggers a export of all the groups spread across the entire cluster. Each server writes output in gzipped rdf to the export directory specified on startup by `--export`. If any of the groups fail, the entire export process is considered failed, and an error is returned.

{{% notice "note" %}}It is up to the user to retrieve the right export files from the servers in the cluster. Dgraph does not copy files  to the server that initiated the export.{{% /notice %}}

## Shutdown

A clean exit of a single dgraph node is initiated by running the following command on that node.
{{% notice "warning" %}}This won't work if called from outside the server where dgraph is running.
{{% /notice %}}

```sh
$ curl localhost:8080/admin/shutdown
```

This stops the server on which the command is executed and not the entire cluster.

## Delete database

Individual triples, patterns of triples and predicates can be deleted as described in the [query languge docs]({{< relref "query-language/index.md#delete" >}}).

To drop all data, you could send a `DropAll` request via `/alter` endpoint.

Alternatively, you could:

* [stop Dgraph]({{< relref "#shutdown" >}}) and wait for all writes to complete,
* delete (maybe do an export first) the `p` and `w` directories, then
* restart Dgraph.

## Upgrade Dgraph

Doing periodic exports is always a good idea. This is particularly useful if you wish to upgrade Dgraph or reconfigure the sharding of a cluster. The following are the right steps safely export and restart.

- Start an [export]({{< relref "#export">}})
- Ensure it's successful
- Bring down the cluster
- Run Dgraph using new data directories.
- Reload the data via [bulk loader]({{< relref "#Bulk Loader" >}}).
- If all looks good, you can delete the old directories (export serves as an insurance)

These steps are necessary because Dgraph's underlying data format could have changed, and reloading the export avoids encoding incompatibilities.

## Post Installation

Now that Dgraph is up and running, to understand how to add and query data to Dgraph, follow [Query Language Spec]({{< relref "query-language/index.md">}}). Also, have a look at [Frequently asked questions]({{< relref "faq/index.md" >}}).

## Monitoring

Dgraph exposes metrics via `/debug/vars` endpoint in json format. Dgraph doesn't store the metrics and only exposes the value of the metrics at that instant. You can either poll this endpoint to get the data in your monitoring systems or install **[Prometheus](https://prometheus.io/docs/introduction/install/)**. Replace targets in the below config file with the ip of your dgraph instances and run prometheus using the command `prometheus -config.file my_config.yaml`.
```sh
scrape_configs:
  - job_name: "dgraph"
    metrics_path: "/debug/prometheus_metrics"
    scrape_interval: "2s"
    static_configs:
    - targets:
      - 172.31.9.133:8080
      - 172.31.15.230:8080
      - 172.31.0.170:8080
      - 172.31.8.118:8080
```

Install **[Grafana](http://docs.grafana.org/installation/)** to plot the metrics. Grafana runs at port 3000 in default settings. Create a prometheus datasource by following these **[steps](https://prometheus.io/docs/visualization/grafana/#creating-a-prometheus-data-source)**. Import **[grafana_dashboard.json](https://github.com/dgraph-io/benchmarks/blob/master/scripts/grafana_dashboard.json)** by following this **[link](http://docs.grafana.org/reference/export_import/#importing-a-dashboard)**.

## Troubleshooting
Here are some problems that you may encounter and some solutions to try.

### Running OOM (out of memory)

During bulk loading of data, Dgraph can consume more memory than usual, due to high volume of writes. That's generally when you see the OOM crashes.

The recommended minimum RAM to run on desktops and laptops is 16GB. Dgraph can take up to 7-8 GB with the default setting `-memory_mb` set to 4096; so having the rest 8GB for desktop applications should keep your machine humming along.

On EC2/GCE instances, the recommended minimum is 8GB. It's recommended to set `-memory_mb` to half of RAM size.

## See Also

* [Product Roadmap to v1.0](https://github.com/dgraph-io/dgraph/issues/1)
