<div align="center">
  <img alt="Gossamer logo" src="/.github/gossamer_logo.png" width="700" />
</div>
<div align="center">
  <a href="https://www.gnu.org/licenses/gpl-3.0">
    <img alt="License: GPL v3" src="https://img.shields.io/badge/License-GPLv3-blue.svg" />
  </a>
  <a href="https://godoc.org/github.com/ChainSafe/gossamer">
    <img alt="go doc" src="https://godoc.org/github.com/ChainSafe/gossamer?status.svg" />
  </a>
  <a href="https://goreportcard.com/report/github.com/ChainSafe/gossamer">
    <img alt="go report card" src="https://goreportcard.com/badge/github.com/ChainSafe/gossamer" />
  </a>
  <a href="https://travis-ci.org/ChainSafe/gossamer/">
    <img alt="build status" src="https://travis-ci.org/ChainSafe/gossamer.svg?branch=development" />
  </a>
</div>
<div align="center">
  <a href="https://codeclimate.com/github/ChainSafe/gossamer/badges">
    <img alt="maintainability" src="https://api.codeclimate.com/v1/badges/933c7bb58eee9aba85eb/maintainability" />
  </a>
  <a href="https://codeclimate.com/github/ChainSafe/gossamer/test_coverage">
    <img alt="Test Coverage" src="https://api.codeclimate.com/v1/badges/933c7bb58eee9aba85eb/test_coverage" />
  </a>
</div>
<br />

## A Blockchain Framework

Gossamer is an implementation of the [Polkadot Host](https://github.com/w3f/polkadot-spec) - a blockchain framework used to build and run node implementations for different blockchain protocols within the Polkadot ecosystem.

Gossamer includes official node implementations for major networks within the Polkadot ecosystem and makes building node implementations for other networks trivial; blockchain protocols built with any implementation of the Polkadot Host can plug a runtime blob into Gossamer to create an additional node implementation in Go.

For more information about Gossamer and the Polkadot Host, check out [Gossamer Wiki](https://github.com/ChainSafe/gossamer/wiki).

## Package Architecture

Gossamer includes [node implementations](#node-implementations) for major networks within the Polkadot ecosystem, [node services](#node-services) that can be used to build and run node implementations, and a collection of [modular packages](#modular-packages) that can be used to build and run node services and other supporting tools.

### Node Implementations

Gossamer includes node implementations in development for Gossamer Testnet and Kusama Network.

| package           | description |
|-                  |-            |
| `node/gssmr`      | a full node implementation and rpc server for Gossamer Testnet |
| `node/ksmcc`      | a full node implementation and rpc server for Kusama Network |

### Node Services

Gossamer includes node services used to build and run node implementations with a shared base protocol (currently each node implementation uses a set of shared node services that make up the base implementation for the Polkadot Host).

| package           | description |
|-                  |-            |
| `dot/core`        | orchestrate service interactions |
| `dot/network`     | peer-to-peer service using libp2p |
| `dot/rpc`         | optional service for RPC server |
| `dot/state`       | storage service for chain state |

### Modular Packages

Gossamer includes a collection of modular packages used to build and run node services and other supporting tools.

| package           | description |
|-                  |-            |
| `lib/babe`        | BABE implementation |
| `lib/blocktree`   | blocktree implementation |
| `lib/common`      | common types and functions |
| `lib/crypto`      | crypto keypair implementations |
| `lib/database`    | generic database using badgerDB |
| `lib/grandpa`     | GRANDPA implementation |
| `lib/keystore`    | library for managing keystore |
| `lib/runtime`     | WASM runtime integration using Wasmer |
| `lib/scale`       | SCALE encoding and decoding |
| `lib/services`    | common interface for node services |
| `lib/transaction` | library for transaction queue |
| `lib/trie`        | modified merkle-patricia trie implementation |

## Running Gossamer

The `gossamer` command can be used to run node implementations that are within this repository.

### Prerequisites

go 1.13.7

### Installation

```
go get -u github.com/ChainSafe/gossamer
```

### Build Gossamer

```
make gossamer
```

### Run Gossamer Node

```
./bin/gossamer init
```

```
./bin/gossamer
```

## Contribute

- Check out [Contributing Guidelines](.github/CONTRIBUTING.md)  
- Have questions? Say hi on [Discord](https://discord.gg/Xdc5xjE)!

## Donate

Our work on gossamer is funded by grants. If you'd like to donate, you can send us ETH or DAI at the following address:
`0x764001D60E69f0C3D0b41B0588866cFaE796972c`

## License

_GNU Lesser General Public License v3.0_

<br />
<p align="center">
	<img src=".github/gopher.png">
</p>
