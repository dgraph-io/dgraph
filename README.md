<div align="center">
  <img alt="Gossamer logo" src="/docs/assets/gossamer_logo.png" width="700" />
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

Gossamer is an implementation of the [Polkadot Host](https://github.com/w3f/polkadot-spec) - a blockchain framework used to build and run node implementations for different blockchain protocols within the Polkadot ecosystem.

Gossamer includes official node implementations for major blockchains within the Polkadot ecosystem and makes building node implementations for other blockchains trivial; blockchains built with [Substrate](https://github.com/paritytech/substrate) can plug their compiled runtime into Gossamer to create a node implementation in Go.

For more information about Gossamer, the Polkadot ecosystem, and how to use Gossamer to build and run nodes for different blockchain protocols within the Polkadot ecosystem, check out [Gossamer Wiki](https://github.com/ChainSafe/gossamer/wiki).

## Get Started

### Prerequisites

install go version `1.13.7`

### Installation

get the [ChainSafe/gossamer](https://github.com/ChainSafe/gossamer) repository:
```
go get -u github.com/ChainSafe/gossamer
```

### Build Command

build gossamer node:
```
make gossamer
```

### Run Default Node

initialize default node:
```
./bin/gossamer --key alice init
```

start default node:
```
./bin/gossamer --key alice
```

### Run Gossamer Node

initialize gossamer node:
```
./bin/gossamer --chain gssmr --key alice init
```

start gossamer node:
```
./bin/gossamer --chain gssmr --key alice
```

### Run Kusama Node

initialize kusama node:
```
./bin/gossamer --chain ksmcc --key alice init
```

start kusama node:
```
./bin/gossamer --chain ksmcc --key alice
```

### Run Tests

run all package tests:
```
go test ./... -short
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
	<img src="/docs/assets/gopher.png">
</p>
