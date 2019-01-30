# systemd Integration for CentOS

The following document describes how to manage `dgraph` with `systemd`.

First, create a system account for `dgraph` service:

```
groupadd --system dgraph
useradd --system -d /var/run/dgraph -s /bin/false -g dgraph dgraph
mkdir -p /var/log/dgraph
mkdir -p /var/run/dgraph/{p,w,zw}
chown -R dgraph:dgraph /var/{run,log}/dgraph
```

Next, copy the `systemd` unit files, i.e. `dgraph.service`, `dgraph-zero.service`,
and `dgraph-ui.service`, in this directory to `/usr/lib/systemd/system/`.

> **NOTE** These unit files expect that Dgraph is installed as `/usr/local/bin/dgraph`.

```
cp dgraph.service /usr/lib/systemd/system/
cp dgraph-zero.service /usr/lib/systemd/system/
cp dgraph-ui.service /usr/lib/systemd/system/
```

Next, enable and start the `dgraph` service. Systemd will also automatically start the
`dgraph-zero` service as a prerequisite.

```
systemctl enable dgraph
systemctl start dgraph
```

The `dgraph-ui` service is optional and, unlike `dgraph-zero`, will not be started
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

> **NOTE** When `dgraph` exits with an error, `systemctl status` may not show the entire error
 output. In that case it may be necessary to use `journald`.
