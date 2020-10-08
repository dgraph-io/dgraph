+++
date = "2017-03-20T22:25:17+11:00"
title = "Config"
weight = 2
[menu.main]
    parent = "deploy"
+++

{{% notice "tip" %}}
For a single server setup, recommended for new users, please see [Get Started]({{< relref "get-started/index.md" >}}) page.
{{% /notice %}}

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
  "postings": "/path/to/p",
  "wal": "/path/to/w"
}
```

Example TOML config file (config.toml):

```toml
my = "localhost:7080"
zero = "localhost:5080"
postings = "/path/to/p"
wal = "/path/to/w"
```


Example YAML config file (config.yml):

```yaml
my: "localhost:7080"
zero: "localhost:5080"
postings: "/path/to/p"
wal: "/path/to/w"
```

Example HCL config file (config.hcl):

```hcl
my = "localhost:7080"
zero = "localhost:5080"
postings = "/path/to/p"
wal = "/path/to/w"
```

Example Java properties config file (config.properties):
```text
my=localhost:7080
zero=localhost:5080
postings=/path/to/p
wal=/path/to/w
```
