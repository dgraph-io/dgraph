---
layout: default
title: Command-Line
permalink: /command-line/
---

- _TODO: update "Command-Line" document [#868](https://github.com/ChainSafe/gossamer/issues/868)_

---

## Gossamer Command

The `gossamer` command is the root command for the `gossamer` package (`cmd/gossamer`). The root command starts the node (and initializes the node if the node has not already been initialized). 

The `gossamer` command accepts the following ***local flags*** and ***global flags***:

```
--log value        Supports levels crit (silent) to trce (trace) (default: "info")
--name value       Node implementation name
--chain value      Node implementation id used to load default node configuration
--config value     TOML configuration file
--base-path value  Data directory for the node
--key value        Specify a test keyring account to use: eg --key=alice
--unlock value     Unlock an account. eg. --unlock=0,2 to unlock accounts 0 and 2. Can be used with --password=[password] to avoid prompt. For multiple passwords, do --password=password1,password2
--port value       Set network listening port (default: 0)
--bootnodes value  Comma separated enode URLs for network discovery bootstrap
--protocol value   Set protocol id
--roles value      Roles of the gossamer node
--nobootstrap      Disables network bootstrapping (mdns still enabled)
--nomdns           Disables network mdns discovery
--rpc              Enable the HTTP-RPC server
--rpchost value    HTTP-RPC server listening hostname
--rpcport value    HTTP-RPC server listening port (default: 0)
--rpcmods value    API modules to enable via HTTP-RPC, comma separated list
--ws               Enable the websockets server
--wsport value     Websockets server listening port (default: 0)
--help, -h         show help
--version, -v      print the version
```

### Accepted Formats

```
gossamer [--global-flags] [--local-flags]
```

```
gossamer [--local-flags] [--global-flags] 
```

## Gossamer Subcommands

List of available ***subcommands***:

```
SUBCOMMANDS:
    help, h     Shows a list of commands or help for one command
    account     Create and manage node keystore accounts
    export      Export configuration values to TOML configuration file
    init        Initialize node databases and load genesis data to state
```

List of ***local flags*** for `init` subcommand:

```
--force            Disable all confirm prompts (the same as answering "Y" to all)
--genesis value    Path to genesis JSON file
--log value        Supports levels crit (silent) to trce (trace) (default: "info")
--name value       Node implementation name
--chain value      Node implementation id used to load default node configuration
--config value     TOML configuration file
--base-path value  Data directory for the node
```

List of ***local flags*** for `account` subcommand:

```
--generate         Generate a new keypair. If type is not specified, defaults to sr25519
--password value   Password used to encrypt the keystore. Used with --generate or --unlock
--import value     Import encrypted keystore file generated with gossamer
--list             List node keys
--ed25519          Specify account type as ed25519
--sr25519          Specify account type as sr25519
--secp256k1        Specify account type as secp256k1
--log value        Supports levels crit (silent) to trce (trace) (default: "info")
--name value       Node implementation name
--chain value      Node implementation id used to load default node configuration
--config value     TOML configuration file
--base-path value  Data directory for the node
```

List of ***local flag*** options for `export` subcommand:

```
--force            Disable all confirm prompts (the same as answering "Y" to all)
--genesis value    Path to genesis JSON file
--log value        Supports levels crit (silent) to trce (trace) (default: "info")
--name value       Node implementation name
--chain value      Node implementation id used to load default node configuration
--config value     TOML configuration file
--base-path value  Data directory for the node
--key value        Specify a test keyring account to use: eg --key=alice
--unlock value     Unlock an account. eg. --unlock=0,2 to unlock accounts 0 and 2. Can be used with --password=[password] to avoid prompt. For multiple passwords, do --password=password1,password2
--port value       Set network listening port (default: 0)
--bootnodes value  Comma separated enode URLs for network discovery bootstrap
--protocol value   Set protocol id
--roles value      Roles of the gossamer node
--nobootstrap      Disables network bootstrapping (mdns still enabled)
--nomdns           Disables network mdns discovery
--rpc              Enable the HTTP-RPC server
--rpchost value    HTTP-RPC server listening hostname
--rpcport value    HTTP-RPC server listening port (default: 0)
--rpcmods value    API modules to enable via HTTP-RPC, comma separated list
--ws               Enable the websockets server
--wsport value     Websockets server listening port (default: 0)
```

### Accepted Formats

```
gossamer [--global-flags] [subcommand] [--local-flags]
```

```
gossamer [subcommand] [--global-flags] [--local-flags]
```

```
gossamer [subcommand] [--local-flags] [--global-flags]
```

### Invalid Formats

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

## Running Node Roles

Run an authority node:
```
./bin/gossamer --key alice --roles 4
```

Run a non-authority node:
```
./bin/gossamer --key alice --roles 1
```

## Running Multiple Nodes

Two options for running another node at the same time...

(1) copy the config file at `cfg/gssmr/config.toml` and manually update `port` and `base-path`:
```
cp cfg/gssmr/config.toml cfg/gssmr/bob.toml
# open bob.toml, set port=7002 and base-path=~/.gossamer/gssmr-bob
# set roles=4 to also make bob an authority node, or roles=1 to make bob a non-authority node
./bin/gossamer --key bob --config cfg/gssmr/bob.toml
```

or run with port, base-path flags:
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
./bin/gossmer --key ferdie
./bin/gossmer --key george
./bin/gossmer --key heather
```

## Initializing Nodes

To initialize or re-initialize a node, use the init subcommand `init`:
```
./bin/gossamer init
./bin/gossamer --key alice --roles 4
```

`init` can be used with the `--base-path` or `--config` flag to re-initialize a custom node (ie, `bob` from the example above):
```
./bin/gossamer --config node/gssmr/bob.toml init
```

## Export Configuration

`export` can be used with the `gossamer` root command-line and `--config` as the export path to export a toml configuration file.
