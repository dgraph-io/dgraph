# Systemd Configuration for a HA Dgraph Cluster

This following document describes how to configure several nodes that are managed through
[systemd](https://systemd.io/).

## Overview

You will configure the following types of Dgraph nodes:

- zero nodes
  - zero leader node - an initial leader node configured at start of cluster, e.g. `zero-0`
  - zero peer nodes - peer nodes, e.g. `zero-1`, `zero-2`, that point to the zero leader
- alpha nodes - configured similarly, e.g. `alpha-0`, `alpha-1`, `alpha-2`, that point to list of
  all zero nodes

> **NOTE** These commands are run as root using bash shell.

## All Nodes (Zero and Alpha)

On all systems that will run a Dgraph service, create `dgraph` group and user.

```bash
groupadd --system dgraph
useradd --system --home-dir /var/lib/dgraph --shell /bin/false --gid dgraph dgraph
```

## All Zero Nodes (Leader and Peers)

On all Zero Nodes, create the these directory paths that are owned by `dgraph` user:

```bash
mkdir --parents /var/{log/dgraph,lib/dgraph/zw}
chown --recursive dgraph:dgraph /var/{lib,log}/dgraph
```

### Configure First Zero Node

Edit the file [dgraph-zero-0.service](dgraph-zero-0.service) as necessary. There are three
parameters and include the hostname:

- `--replicas` - total number of zeros
- `--idx` - initial zero node will be `1`, and each zero node added afterward will have the `idx`
  increased by `1`

Copy the file to `/etc/systemd/system/dgraph-zero.service` and run the following:

```bash
systemctl enable dgraph-zero
systemctl start dgraph-zero
```

### Configure Second Zero Node

This process is similar to previous step. Edit the file
[dgraph-zero-1.service](dgraph-zero-1.service) as required. Replace the string `{{ zero-0 }}` to
match the hostname of the zero leader, such as `zero-0`. The `idx` will be set to `2`

Copy the file to `/etc/systemd/system/dgraph-zero.service` and run the following:

```bash
systemctl enable dgraph-zero
systemctl start dgraph-zero
```

### Configure Third Zero Node

For the third zero node, [dgraph-zero-2.service](dgraph-zero-2.service), this is configured in the
same manner as the second zero node with the `idx` set to `3`

Copy the file to `/etc/systemd/system/dgraph-zero.service` and run the following:

```bash
systemctl enable dgraph-zero
systemctl start dgraph-zero
```

### Configure Firewall for Zero Ports

For zero you will want to open up port `5080` (GRPC). The port `6080` (HTTP) is optional admin port
that is not required by clients. For further information, see
https://dgraph.io/docs/deploy/ports-usage/. This process will vary depending on firewall you are
using. Some examples below:

On **Ubuntu 18.04**:

```bash
# enable internal port
ufw allow from any to any port 5080 proto tcp
# admin port (not required by clients)
ufw allow from any to any port 6080 proto tcp
```

On **CentOS 8**:

```bash
# NOTE: public zone is the default and includes NIC used to access service
# enable internal port
firewall-cmd --zone=public --permanent --add-port=5080/tcp
# admin port (not required by clients)
firewall-cmd --zone=public --permanent --add-port=6080/tcp
firewall-cmd --reload
```

## Configure Alpha Nodes

On all Alpha Nodes, create the these directory paths that are owned by `dgraph` user:

```bash
mkdir --parents /var/{log/dgraph,lib/dgraph/{w,p}}
chown --recursive dgraph:dgraph /var/{lib,log}/dgraph
```

Edit the file [dgraph-alpha.service](dgraph-alpha.service) as required. For the `--zero` parameter,
you want to create a list that matches all the zeros in your cluster, so that when `{{ zero-0 }}`,
`{{ zero-1 }}`, and `{{ zero-2 }}` are replaced, you will have a string something like this
(adjusted to your organization's domain):

```bash
--zero zero-0:5080,zero-1:5080,zero-2:5080
```

Copy the edited file to `/etc/systemd/system/dgraph-alpha.service` and run the following:

```bash
systemctl enable dgraph-alpha
systemctl start dgraph-alpha
```

### Configure Firewall for Alpha Ports

For alpha you will want to open up ports `7080` (GRPC), `8080` (HTTP/S), and `9080` (GRPC). For
further information, see: https://dgraph.io/docs/deploy/ports-usage/. This process will vary
depending on firewall you are using. Some examples below:

On **Ubuntu 18.04**:

```bash
# enable internal ports
ufw allow from any to any port 7080 proto tcp
# enable external ports
ufw allow from any to any port 8080 proto tcp
ufw allow from any to any port 9080 proto tcp
```

On **CentOS 8**:

```bash
# NOTE: public zone is the default and includes NIC used to access service
# enable internal port
firewall-cmd --zone=public --permanent --add-port=7080/tcp
# enable external ports
firewall-cmd --zone=public --permanent --add-port=8080/tcp
firewall-cmd --zone=public --permanent --add-port=9080/tcp
firewall-cmd --reload
```

## Verifying Services

Below are examples of checking the health of the nodes and cluster.

> **NOTE** Replace hostnames to your domain or use the IP address.

### Zero Nodes

You can check the health and state endpoints of the service:

```bash
curl zero-0:6080/health
curl zero-0:6080/state
```

On the system itself, you can check the service status and logs:

```bash
systemctl status dgraph-zero
journalctl -u dgraph-zero
```

### Alpha Nodes

You can check the health and state endpoints of the service:

```bash
curl alpha-0:8080/health
curl alpha-0:8080/state
```

On the system itself, you can check the service status and logs:

```bash
systemctl status dgraph-alpha
journalctl -u dgraph-alpha
```
