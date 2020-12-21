+++
date = "2017-03-20T22:25:17+11:00"
title = "Multi Host Setup"
weight = 5
[menu.main]
    parent = "deploy"
+++

## Using Docker Swarm

### Cluster Setup Using Docker Swarm

{{% notice "note" %}}These instructions are for running Dgraph Alpha without TLS config.
Instructions for running with TLS refer [TLS instructions]({{< relref "deploy/tls-configuration.md" >}}).{{% /notice %}}

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

## HA Cluster setup using Docker Swarm

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
