+++
date = "2017-03-20T22:25:17+11:00"
title = "Deploy"
[menu.main]
  url = "/deploy/"
  identifier = "deploy"
  weight = 8
+++

This page talks about running Dgraph in various deployment modes, in a distributed fashion and involves
running multiple instances of Dgraph, over multiple servers in a cluster.

{{% notice "tip" %}}
For a single server setup, recommended for new users, please see [Get Started]({{< relref "get-started/index.md" >}}) page.
{{% /notice %}}

## Docker

```sh
docker pull dgraph/dgraph:latest

# You can test that it worked fine, by running:
docker run -it dgraph/dgraph:latest dgraph
```

## Automatic download

Running

```sh
curl https://get.dgraph.io -sSf | bash

# Test that it worked fine, by running:
dgraph
```

would install the `dgraph` binary into your system.

Other instalation options:

> Add `-s --` before the flags.()
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

## Manual download [optional]

If you don't want to follow the automatic installation method, you could manually download the appropriate tar for your platform from **[Dgraph releases](https://github.com/dgraph-io/dgraph/releases)**. After downloading the tar for your platform from Github, extract the binary to `/usr/local/bin` like so.

```sh
# For Linux
$ sudo tar -C /usr/local/bin -xzf dgraph-linux-amd64-VERSION.tar.gz

# For Mac
$ sudo tar -C /usr/local/bin -xzf dgraph-darwin-amd64-VERSION.tar.gz

# Test that it worked fine, by running:
dgraph
```

## Building from Source

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

## Config

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

## See Also

* [Product Roadmap 2020](https://github.com/dgraph-io/dgraph/issues/4724)
