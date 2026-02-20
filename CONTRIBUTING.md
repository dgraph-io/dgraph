# Contributing to Dgraph

- [Getting Started](#getting-started)
- [Setting Up the Development Environment](#setting-up-the-development-environment)
  - [Prerequisites](#prerequisites)
  - [Setup Dgraph from source repo](#setup-dgraph-from-source-repo)
  - [Setup Badger from source repo](#setup-badger-from-source-repo-optional)
  - [Protocol buffers](#protocol-buffers)
  - [Build Dgraph](#build-dgraph)
  - [Build Docker Image](#build-docker-image)
  - [Testing](#testing)
- [Contributing](#contributing)
  - [Guidelines](#guidelines)
  - [Code style](#code-style)
  - [License Header](#license-header)
  - [Signed Commits](#signed-commits)

## Getting Started

- Read the [Getting Started Guide](https://dgraph.io/docs/get-started/)
- [Take the Dgraph tour](https://dgraph.io/tour/)

## Setting Up the Development Environment

### Prerequisites

- Install [Git](https://git-scm.com/) (may be already installed on your system, or available through
  your OS package manager)
- Install [Make](https://www.gnu.org/software/make/) (may be already installed on your system, or
  available through your OS package manager)
- Install [Docker](https://docs.docker.com/install/) and
  [Docker Compose](https://docs.docker.com/compose/install/).
- [Install Go 1.24.3 or above](https://golang.org/doc/install).
- Install
  [trunk](https://docs.trunk.io/code-quality/overview/getting-started/install#install-the-launcher).
  Our CI uses trunk to lint and check code, having it installed locally will save you time.

### Setup Dgraph from source repo

```bash
git clone https://github.com/dgraph-io/dgraph.git
cd ./dgraph
make install
```

This will put the source code in a Git repo under `$GOPATH/src/github.com/dgraph-io/dgraph` and
compile the binaries to `$GOPATH/bin`.

### Setup Badger from source repo (optional)

Dgraph source repo vendors its own version of Badger. If you are just working on Dgraph, you do not
necessarily need to check out Badger from its own repo. However, if you want to contribute to Badger
as well, you will need to check it out from its own repo.

```bash
go get -t -v github.com/dgraph-io/badger
```

This will put the source code in a Git repo under `$GOPATH/src/github.com/dgraph-io/badger`.

### Protocol buffers

We use [protocol buffers](https://developers.google.com/protocol-buffers/) to serialize data between
our server and our clients and also for inter-worker communication. If you make any changes to the
`.proto` files, you would have to recompile them.

Install the `protoc` compiler which is required for compiling proto files used for gRPC
communication. Get `protoc` version 3.0.0 or above from
[GitHub releases page](https://github.com/google/protobuf/releases/latest) (look for the binary
releases at the bottom, or compile from sources
[following the instructions](https://github.com/google/protobuf/tree/main/src)).

We use [gogo protobuf](https://github.com/gogo/protobuf) in Dgraph. To get the protocol buffer
compiler plugin from gogo run

```bash
go get -u github.com/gogo/protobuf/protoc-gen-gofast
```

To compile the proto file using the `protoc` plugin and the gogo compiler plugin run the command
`make regenerate` from within the directory containing the `.proto` files.

```bash
cd protos
make regenerate
```

This should generate the required `.pb.go` file.

### Build Dgraph

You can build Dgraph using `make dgraph` or `make install` which will add the version information to
the binary.

- `make dgraph`: Creates a `dgraph` binary at `./dgraph/dgraph`
- `make install`: Creates a `dgraph` binary at `$GOPATH/bin/dgraph`. You should add `$GOPATH/bin` to
  your `$PATH` if it isn't there already.

```sh
$ make install
Installing Dgraph...
Commit SHA256: 15839b156e9920ca2c4ab718e1e73b6637b8ecec
Old SHA256: 596e362ede7466a2569d19ded91241e457e665ada785d05a902af2c6f2cea508
Installed dgraph to /Users/<homedir>/go/bin/dgraph

$ dgraph version
Dgraph version   : v24.0.2-103-g15839b156
Dgraph codename  : dgraph
Dgraph SHA-256   : 9ce738cd055dfebdef5d68b2a49ea4e062e597799498607dbd1bb618d48861a6
Commit SHA-1     : 15839b156
Commit timestamp : 2025-01-10 17:56:49 -0500
Branch           : username/some-branch-that-im-on
Go version       : go1.22.12
jemalloc enabled : true

For Dgraph official documentation, visit https://dgraph.io/docs.
For discussions about Dgraph     , visit https://discuss.dgraph.io.

Licensed variously under the Apache Public License 2.0 and Dgraph Community License.
© Istari Digital, Inc.
```

#### Building Dgraph on non-Linux machines

See the [README](t/README.md) in the [_t_](t) folder for instructions on building Dgraph on
non-Linux machines.

### Build Docker Image

```sh
make image-local
```

To build a test Docker image from source, use `make image-local`. This builds a linux-compatible
Dgraph binary using `make dgraph` and creates a Docker image tagged `dgraph/dgraph:local`. You can
then use this local image to test Dgraph in your local Docker setup.

### Testing

Dgraph employs a ~~complex~~ sophisticated testing framework with extensive test coverage. A full
test run can take several hours. We've developed a custom test runner in Go in the [t/](t)
directory, providing control and flexibility beyond the standard Go testing framework.

The simplest way to run tests is via Make:

```bash
# Run all tests
make test

# Run specific test types
make test-unit          # Unit tests only (no Docker)
make test-integration2  # Integration2 tests via dgraphtest
make test-upgrade       # Upgrade tests

# Use variables for more control
make test TAGS=integration2 PKG=systest/vector
make test TIMEOUT=90m          # Override per-package timeout (default: 30m)
```

Run `make help` to see all available targets and variables.

For a comprehensive testing guide, see [TESTING.md](TESTING.md).

## Contributing

### Guidelines

Over years of writing big scalable systems, we are convinced that striving for simplicity wherever
possible is the only way to build robust systems. This simplicity could be in design, could be in
coding, or could be achieved by rewriting an entire module, that you may have painstakingly finished
yesterday.

- **Pull requests are welcome**, as long as you're willing to put in the effort to meet the
  guidelines. After you fork Dgraph, create your pull request against our `main` branch. Please
  follow the instructions in the PR template carefully.
- Be prepared to document your additions/changes in our public documentation (if applicable)
- Aim for clear, well written, maintainable code
- Simple and minimal approach to features, like Go
- New features must include passing unit tests, and integration tests when appropriate
- Refactoring existing code now for better performance, better readability or better testability
  wins over adding a new feature
- Don't add a function to a module that you don't use right now, or doesn't clearly enable a planned
  functionality
- Don't ship a half done feature, which would require significant alterations to work fully
- Avoid [Technical debt](https://en.wikipedia.org/wiki/Technical_debt) like cancer
- Leave the code cleaner than when you began

### Code style

- We're following [Go Code Review](https://github.com/golang/go/wiki/CodeReviewComments)
- At a minimum, use `go fmt` to format your code before committing. Ideally you should use `trunk`
  as our CI will run `trunk` on your code
- If you see _any code_ which clearly violates the style guide, please fix it and send a pull
  request. No need to ask for permission
- Avoid unnecessary vertical spaces. Use your judgment or follow the code review comments
- Wrap your code and comments to 120 characters, unless doing so makes the code less legible

### License Header

Every new source file must begin with a license header.

Most of Dgraph, Badger, and the Dgraph clients (dgo, dgraph-js, pydgraph and dgraph4j) are licensed
under the Apache 2.0 license:

```sh
/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */
```

### Signed Commits

Signed commits help in verifying the authenticity of the contributor. We use signed commits in
Dgraph, and we prefer it, though it's not compulsory to have signed commits. This is a recommended
step for people who intend to contribute to Dgraph on a regular basis.

Follow instructions to generate and setup GPG keys for signing code commits on this
[Github Help page](https://help.github.com/articles/signing-commits-with-gpg/).
