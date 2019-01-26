# Contributing to Dgraph

* [Getting Started](#getting-started)
* [Setting Up the Development Environment](#setting-up-the-development-environment)
   * [Prerequisites](#prerequisites)
   * [Setup Dgraph from source repo](#setup-dgraph-from-source-repo)
   * [Setup Badger from source repo](#setup-badger-from-source-repo)
   * [Protocol buffers](#protocol-buffers)
   * [Testing](#testing)
* [Doing a release](#doing-a-release)
* [Contributing](#contributing)
   * [Guidelines](#guidelines)
   * [Code style](#code-style)
   * [License Header](#license-header)
   * [Signed Commits](#signed-commits)

## Getting Started

- Read the [Getting Started Guide](https://docs.dgraph.io/get-started/)
- [Take the Dgraph tour](https://tour.dgraph.io)

## Setting Up the Development Environment

### Prerequisites

- Install [Git](https://git-scm.com/) (may be already installed on your system, or available through your OS package manager)
- [Install Go 1.8 or above](https://golang.org/doc/install)

### Setup Dgraph from source repo

    $ go get -v -t github.com/dgraph-io/dgraph/...

This will put the source code in a Git repo under `$GOPATH/src/github.com/dgraph-io/dgraph` and compile the binaries to `$GOPATH/bin`.

### Setup Badger from source repo

Dgraph source repo vendors its own version of Badger. If you are just working on Dgraph, you do not necessarily need to check out Badger from its own repo. However, if you want to contribute to Badger as well, you will need to check it out from its own repo.


    $ go get -t -v github.com/dgraph-io/badger

This will put the source code in a Git repo under `$GOPATH/src/github.com/dgraph-io/badger`.

### Protocol buffers

We use [protocol buffers](https://developers.google.com/protocol-buffers/) to serialize data between our server and the Go client and also for inter-worker communication. If you make any changes to the `.proto` files, you would have to recompile them. 

Install the `protoc` compiler which is required for compiling proto files used for gRPC communication. Get `protoc` version 3.0.0 or above from [GitHub releases page](https://github.com/google/protobuf/releases/latest) (look for the binary releases at the bottom, or compile from sources [following the instructions](https://github.com/google/protobuf/tree/master/src)).

We use [gogo protobuf](https://github.com/gogo/protobuf) in Dgraph. To get the protocol buffer compiler plugin from gogo run


    $ go get -u github.com/gogo/protobuf/protoc-gen-gofast

To compile the proto file using the `protoc` plugin and the gogo compiler plugin run the script `gen.sh` from within the directory containing the `.proto` files.


    $ cd protos
    $ ./gen.sh

This should generate the required `.pb.go` file.

### Testing

#### Dgraph
Run the `test.sh` script in the root folder.


    $ ./test.sh
    
    INFO: Running tests using the default cluster
    …
    INFO: Running test for github.com/dgraph-io/dgraph/algo
    ok  	github.com/dgraph-io/dgraph/algo	0.004s
    INFO: Running test for github.com/dgraph-io/dgraph/codec
    ok  	github.com/dgraph-io/dgraph/codec	9.308s
    INFO: Running test for github.com/dgraph-io/dgraph/codec/benchmark
    ?   	github.com/dgraph-io/dgraph/codec/benchmark	[no test files]
    …

Run `test.sh --help` for more info.

Tests should be written in Go and use the Dgraph cluster set up in `dgraph/docker-compose.yml`
whenever possible. If the functionality being tested requires a different cluster setup (e.g.
different commandline options), the `*_test.go` files should be put in a separate directory that
also contains a `docker-compose.yml` to set up the cluster as needed.

 **IMPORTANT:** All containers should be labeled with `cluster: test` so they may be correctly
 restarted and cleaned up by the test script.

#### Badger
Run `go test` in the root folder.


    $ go test ./...
    ok      github.com/dgraph-io/badger     24.853s
    ok      github.com/dgraph-io/badger/skl 0.027s
    ok      github.com/dgraph-io/badger/table       0.478s
    ok      github.com/dgraph-io/badger/y   0.004s

## Doing a release

* Create a branch called `release/v<x.y.z>` from master. For e.g. `release/v1.0.5`. Look at the
   diff between the last release and master and make sure that `CHANGELOG.md` has all the changes
   that went in. Also make sure that any new features/changes are added to the docs under
   `wiki/content` to the relevant section.
* Test any new features or bugfixes and then tag the final commit on the release branch like:

  ```sh
  git tag -s -a v1.0.5
  ```

* Push the release branch and the tagged commit.

  ```sh
  git push origin release/v<x.y.z>
  git push origin v<x.y.z>
  ```

* Travis CI would run the `contrib/nightly/upload.sh` script when a new tag is pushed. This script
  would create the binaries for `linux`, `darwin` and `windows` and also upload them to Github after
  creating a new draft release. It would also publish a new docker image for the new release as well
  as update the docker image with tag `latest` and upload them to docker hub.

* Checkout the `master` branch and merge the tag to it and push it.

  ```sh
  git checkout master
  git merge v<x.y.z>
  git push origin master
  ```

* Once the draft release is published on Github by Travis, modify it to add the release notes. The release
  notes would mostly be the same as changes for the current version in `CHANGELOG.md`. Finally publish the 
  release and announce to users on community Slack.

* To make sure that docs are added for the newly released version, add the version to
   `wiki/scripts/build.sh`. It is also important for a release branch for the version to exist,
   otherwise docs won't be built and published for it. SSH into the server serving the docs and pull
   the latest version of `wiki/scripts/build.sh` from master branch and rerun it so that it can start
   publishing docs for the latest version.

* If any bugs were fixed with regards to query language or in the server then it is a good idea to
  deploy the latest version on `play.dgraph.io`.

## Contributing

### Guidelines

Over years of writing big scalable systems, we are convinced that striving for simplicity wherever possible is the only way to build robust systems. This simplicity could be in design, could be in coding, or could be achieved by rewriting an entire module, that you may have painstakingly finished yesterday.


- **Pull requests are welcome**, as long as you're willing to put in the effort to meet the guidelines.
- Aim for clear, well written, maintainable code.
- Simple and minimal approach to features, like Go.
- Refactoring existing code now for better performance, better readability or better testability wins over adding a new feature.
- Don't add a function to a module that you don't use right now, or doesn't clearly enable a planned functionality.
- Don't ship a half done feature, which would require significant alterations to work fully.
- Avoid [Technical debt](https://en.wikipedia.org/wiki/Technical_debt) like cancer.
- Leave the code cleaner than when you began.

### Code style
- We're following [Go Code Review](https://github.com/golang/go/wiki/CodeReviewComments).
- Use `go fmt` to format your code before committing.
- If you see *any code* which clearly violates the style guide, please fix it and send a pull request. No need to ask for permission.
- Avoid unnecessary vertical spaces. Use your judgment or follow the code review comments.
- Wrap your code and comments to 100 characters, unless doing so makes the code less legible.

### License Header

Every new source file must begin with a license header.

Most of Dgraph, Badger, and the Dgraph clients (dgo, dgraph-js, pydgraph and dgraph4j) are licensed under the Apache 2.0 license:

    /*
     * Copyright 2016-2018 Dgraph Labs, Inc. and Contributors
     *
     * Licensed under the Apache License, Version 2.0 (the "License");
     * you may not use this file except in compliance with the License.
     * You may obtain a copy of the License at
     *
     *    http://www.apache.org/licenses/LICENSE-2.0
     *
     * Unless required by applicable law or agreed to in writing, software
     * distributed under the License is distributed on an "AS IS" BASIS,
     * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
     * See the License for the specific language governing permissions and
     * limitations under the License.
     */

### Signed Commits

Signed commits help in verifying the authenticity of the contributor. We use signed commits in Dgraph, and we prefer it, though it's not compulsory to have signed commits. This is a recommended step for people who intend to contribute to Dgraph on a regular basis.

Follow instructions to generate and setup GPG keys for signing code commits on this [Github Help page](https://help.github.com/articles/signing-commits-with-gpg/).

