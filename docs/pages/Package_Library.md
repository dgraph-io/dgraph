---
layout: default
title: Package Library
permalink: /package-library/
---

## Overview

Gossamer is a ***modular blockchain framework***; it was designed with a package structure that makes it possible to reuse Gossamer packages to build and run custom nodes and node services.

This document provides an overview of the packages that make up the Gossamer framework - more detailed information about each package can be found at [pkg.go.dev/ChainSafe/gossamer](https://pkg.go.dev/github.com/ChainSafe/gossamer).

Gossamer packages can be categorized into **four package types**:

- **[cmd packages](#cmd-packages)**

    - `cmd/...` - _command packages for running nodes and other services_

- **[host packages](#host-packages)**

    - `host/...` - _the host node package and host node service packages_

- **[lib packages](#lib-packages)**

    - `lib/...` - _modular packages for building nodes and other services_

- **[test packages](#test-packages)**

    - `tests/...` - _test packages for test framework (ie, integration tests)_

## cmd packages

#### `cmd/gossaamer`

- The **gossamer package** is used to run nodes built with Gossamer.

## host packages

#### `host`

- The **host package** implements the shared base protocol for all node implementations operating within the Polkadot ecosystem. The **host package** implements the [Host Node](../host-architecture#host-node); it is the base node implementation for all [Official Nodes](../host-architecture#official-nodes) and [Custom Services](../host-architecture#custom-services) built with Gossamer.

#### `host/core`

- The **core package** implements the [Core Service](../host-architecture#core-service) -  responsible for block production and block finalization (consensus) and processing messages received from the [Network Service](../host-architecture#network-service).

#### `host/network`

- The **network package** implements the [Network Service](../host-architecture#network-service) - responsible for coordinating network host and peer interactions. It manages peer connections, receives and parses messages from connected peers and handles each message based on its type.

#### `host/state`

- The **state package** implements the [State Service](../host-architecture#state-service) - the source of truth for all chain and node state that is made accessible to all node services.

#### `host/rpc`

- The **rpc package** implements the [RPC Service](../host-architecture#rpc-service) - an HTTP server that handles state interactions.

#### `host/types`

- The **types package** implements types defined within the Polkadot Host specification.

## lib packages

#### `lib/babe`

- the **babe package** implements BABE

#### `lib/blocktree`

- the **blocktree package** implements blocktree

#### `lib/common`

- the **common package** defines common types and functions

#### `lib/crypto`

- the **crypto package** implements crypto keypairs

#### `lib/database`

- the **database package** is a generic database built with badgerDB

#### `lib/grandpa`

- the **grandpa package** implements GRANDPA

#### `lib/keystore`

- the **keystore package** manages the keystore and includes test keyrings

#### `lib/runtime`

- the **runtime package** implements the WASM runtime using Wasmer

#### `lib/scale`

- the **scale package** implements the SCALE codec

#### `lib/services`

- the **services package** implements a common interface for node services

#### `lib/transaction`

- the **transaction package** is used to manage transaction queues

#### `lib/trie`

- the **trie package** implements a modified merkle-patricia trie

#### `lib/utils`

- the **utils package** is used to manage node and test directories
