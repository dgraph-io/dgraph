# systemd Integration for CentOS

The following document describes how to manage `dgraph` with `systemd`.

First, you need to install Dgraph:

```Bash
curl https://get.dgraph.io -sSf | bash
```

Then create a system account for `dgraph` service:

> **NOTE** You must run these operations as root.

```Bash
groupadd --system dgraph
useradd --system -d /var/lib/dgraph -s /bin/false -g dgraph dgraph
mkdir -p /var/log/dgraph
mkdir -p /var/lib/dgraph/{p,w,zw}
chown -R dgraph:dgraph /var/{lib,log}/dgraph
```

Next, copy the `systemd` unit files, i.e. `dgraph-alpha.service`, `dgraph-zero.service`, and
`dgraph-ui.service`, in this directory to `/etc/systemd/system/`.

> **NOTE** These unit files expect that Dgraph is installed as `/usr/local/bin/dgraph`.

```Bash
cp dgraph-alpha.service /etc/systemd/system/
cp dgraph-zero.service /etc/systemd/system/
cp dgraph-ui.service /etc/systemd/system/
```

Next, enable and start the `dgraph-alpha` service. Systemd will also automatically start the
`dgraph-zero` service as a prerequisite.

```Bash
systemctl enable dgraph-alpha
systemctl start dgraph-alpha
```

The `dgraph-ui` service is optional and, unlike `dgraph-zero`, will not be started automatically.

```Bash
systemctl enable dgraph-ui
systemctl start dgraph-ui
```

If necessary, create an `iptables` rule to allow traffic to `dgraph-ui` service:

```Bash
iptables -I INPUT 4 -p tcp -m state --state NEW --dport 8000 -j ACCEPT
iptables -I INPUT 4 -p tcp -m state --state NEW --dport 8080 -j ACCEPT
```

Check the status of the services:

```Bash
systemctl status dgraph-alpha
systemctl status dgraph-zero
systemctl status dgraph-ui
```

The logs are available via `journald`:

```Bash
journalctl -u dgraph-zero.service --since today
journalctl -u dgraph-zero.service -r
journalctl -u dgraph-alpha.service -r
journalctl -u dgraph-ui.service -r
```

You can also follow the logs using journalctl -f:

```Bash
journalctl -f -u dgraph-zero.service
journalctl -f -u dgraph-alpha.service
journalctl -f -u dgraph-ui.service
```

> **NOTE** When `dgraph` exits with an error, `systemctl status` may not show the entire error
> output. In that case it may be necessary to use `journald`.
