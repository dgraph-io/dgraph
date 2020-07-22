<div align="center">
  <img alt="Gossamer logo" src="/docs/assets/img/gossamer_banner.png" width="600" />
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

## A Go Implementation of the Polkadot Host

Gossamer is an implementation of the [Polkadot Host](https://github.com/w3f/polkadot-spec): a framework used to build and run nodes for different blockchain protocols that are compatible with the Polkadot ecosystem.  The core of the Polkadot Host is the wasm runtime which handles the logic of the chain.

Gossamer includes node implementations for major blockchains within the Polkadot ecosystem and simplifies building node implementations for other blockchains. Runtimes built with [Substrate](https://github.com/paritytech/substrate) can plug their runtime into Gossamer to create a node implementation in Go.

For more information about Gossamer, the Polkadot ecosystem, and how to use Gossamer to build and run nodes for various blockchain protocols within the Polkadot ecosystem, check out the [Gossamer Docs](https://ChainSafe.github.io/gossamer).

## Get Started

### Prerequisites

install go version `>=1.14`

### Installation

get the [ChainSafe/gossamer](https://github.com/ChainSafe/gossamer) repository:
```
go get -u github.com/ChainSafe/gossamer
```

You may encounter a `package github.com/ChainSafe/gossamer: no Go files in ...` message. This is not an error, since there are no go files in the project root. 

build gossamer command:
```
make gossamer
```

### Run Default Node

initialize default node:
```
./bin/gossamer init
```

start default node:
```
./bin/gossamer --key alice
```

The built-in keys available for the node are `alice`, `bob`, `charlie`, `dave`, `eve`, `ferdie`, `george`, and `ian`.

### Run Gossamer Node

initialize gossamer node:
```
./bin/gossamer --chain gssmr init
```

start gossamer node:
```
./bin/gossamer --chain gssmr --key alice
```

### Run Kusama Node (_in development_)

initialize kusama node:
```
./bin/gossamer --chain ksmcc --key alice init
```

start kusama node:
```
./bin/gossamer --chain ksmcc --key alice
```

### Run Polkadot Node (_in development_)

initialize polkadot node:
```
./bin/gossamer --chain dotcc --key alice init
```

start polkadot node:
```
./bin/gossamer --chain dotcc --key alice
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
	<img src="/docs/assets/img/chainsafe_gopher.png">
</p>
