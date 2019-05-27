 # gossamer
 
 ## Golang Polkadot Runtime Environment Implementation  

[![GoDoc](https://godoc.org/github.com/ChainSafe/gossamer?status.svg)](https://godoc.org/github.com/ChainSafe/gossamer)
[![Go Report Card](https://goreportcard.com/badge/github.com/ChainSafe/gossamer)](https://goreportcard.com/report/github.com/ChainSafe/gossamer)
[![Build Passing](https://img.shields.io/travis/com/ChainSafe/gossamer/development.svg?label=development&logo=travis "Development Branch (Travis)")](https://travis-ci.com/ChainSafe/gossamer)
[![Maintainability](https://api.codeclimate.com/v1/badges/933c7bb58eee9aba85eb/maintainability)](https://codeclimate.com/github/ChainSafe/gossamer/badges)
[![Test Coverage](https://api.codeclimate.com/v1/badges/933c7bb58eee9aba85eb/test_coverage)](https://codeclimate.com/github/ChainSafe/gossamer/test_coverage)
[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](https://www.gnu.org/licenses/gpl-3.0)
[![standard-readme compliant](https://img.shields.io/badge/readme%20style-standard-brightgreen.svg?style=flat-square)](https://github.com/RichardLitt/standard-readme)
[![](https://img.shields.io/twitter/follow/espadrine.svg?label=Follow&style=social)](https://twitter.com/chainsafeth)


## Install

```
go get -u github.com/ChainSafeSystems/gossamer
```

## Usage 

```
make gossamer
gossamer --config config.toml
```

### Docker

To start Gossamer in a docker contaienr, run:

```
make docker
```

#### Running Manually

To build the image, run this command:

```
docker build -t chainsafe/gossamer -f Docerfile.dev
```

Start an instance with:

```
docker run chainsafe/gossamer
```

## Test
```
go test -v ./...
```

## Contributing
- Check out our contribution guidelines: [CONTRIBUTING.md](CONTRIBUTING.md)  
- Have questions? Say hi on [Gitter](https://gitter.im/chainsafe/gossamer)!

## License
_GNU General Public License v3.0_

