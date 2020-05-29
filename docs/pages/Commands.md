---
layout: default
title: Commands
permalink: /commands/
---

_work in progress_ - [gossamer/issues/868](https://github.com/ChainSafe/gossamer/issues/868)

## Accepted formats

```
gossamer [--global-flags] [--local-flags]
```

```
gossamer [--local-flags] [--global-flags] 
```

...

```
gossamer [--global-flags] [subcommand] [--local-flags]
```

```
gossamer [subcommand] [--global-flags] [--local-flags]
```

```
gossamer [subcommand] [--local-flags] [--global-flags]
```

## Invalid formats

_please note that `[--local-flags]` must come after `[subcommand]`_

```
gossamer [--local-flags] [subcommand] [--global-flags] 
```

```
gossamer [--local-flags] [--global-flags] [subcommand] 
```

```
gossamer [--global-flags] [--local-flags] [subcommand] 
```

## Running gossamer

Run an authority node:
```
./bin/gossamer --key alice --roles 4
```

Run a non-authority node:
```
./bin/gossamer --key alice --roles 1
```

Two options for running another node at the same time...

(1) copy the config file at `cfg/gssmr/config.toml` and manually update `port` and `base-path`:
```
cp cfg/gssmr/config.toml cfg/gssmr/bob.toml
# open bob.toml, set port=7002 and base-path=~/.gossamer/gssmr-bob
# set roles=4 to also make bob an authority node, or roles=1 to make bob a non-authority node
./bin/gossamer --key bob --config cfg/gssmr/bob.toml
```

or run with port, datadir flags:
```
./bin/gossmer --key bob --port 7002 --base-path ~/.gossamer/gssmr-bob --roles 4
```

To run more than two nodes, repeat steps for bob with a new `port` and `base-path` replacing `bob`.

Available built-in keys:
```
./bin/gossmer --key alice
./bin/gossmer --key bob
./bin/gossmer --key charlie
./bin/gossmer --key dave
./bin/gossmer --key eve
./bin/gossmer --key fred
./bin/gossmer --key george
./bin/gossmer --key heather
```

To re-initialize a node, use the init subcommand `init`:
```
./bin/gossamer init
./bin/gossamer --key alice --roles 4
```

`init` can be used with the `--base-path` or `--config` flag to re-initialize a custom node (ie, `bob` from the example above):
```
./bin/gossamer --config node/gssmr/bob.toml init
```

`export` can be used with the `gossamer` root commands and `--config` as the export path to export a toml configuration file.
