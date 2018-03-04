# systemd Integration for CentOS

The following document describes how to manage `dgraph` with `systemd`.

First, create a system account for `dgraph` service:

```
groupadd --system dgraph
useradd --system -d /var/run/dgraph -s /bin/bash -g dgraph dgraph
mkdir -p /var/log/dgraph
mkdir -p /var/run/dgraph/{p,w,zw}
chown -R dgraph:dgraph /var/{run,log}/dgraph
```

Next, copy the `systemd` unit files, i.e. `dgraph.service`, `dgraph-zero.service`,
and `dgraph-ui.service`, in this directory to `/usr/lib/systemd/system/`.

```
cp dgraph.service /usr/lib/systemd/system/
cp dgraph-zero.service /usr/lib/systemd/system/
cp dgraph-ui.service /usr/lib/systemd/system/
```

Next, enable and start the `dgraph` services. Please note that when a user
starts the `dgraph` service, the `systemd` starts `dgraph-zero` service
automatically, as a prerequisite.

```
systemctl enable dgraph
systemctl start dgraph
```

The `dgraph-ui` service is optional, and, therefore, it will not start
automatically.

```
systemctl enable dgraph-ui
systemctl start dgraph-ui
```

If necessary, create an `iptables` rule to allow traffic to `dgraph-ui` service:

```
iptables -I INPUT 4 -p tcp -m state --state NEW --dport 8000 -j ACCEPT
iptables -I INPUT 4 -p tcp -m state --state NEW --dport 8080 -j ACCEPT
```

Check the status of the services:

```
systemctl status dgraph
systemctl status dgraph-zero
systemctl status dgraph-ui
```

The logs are available via `journald`:

```
journalctl -u dgraph-zero.service --since today
journalctl -u dgraph-zero.service -r
journalctl -u dgraph.service -r
journalctl -u dgraph-ui.service -r
```
