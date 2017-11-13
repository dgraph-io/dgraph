+++
title = "Contribute to Dgraph"
+++
# Getting Started
- Read the [Getting Started Guide](https://docs.dgraph.io/get-started/)
- [Take the Dgraph tour](https://tour.dgraph.io)

## Setting Up the Development Environment

### Prerequisites

- Install [Git](https://git-scm.com/) (may be already installed on your system, or available through your OS package manager)
- [Install Go 1.8 or above](https://golang.org/doc/install)

### Setup Dgraph from source repo

    $ go get -t -v github.com/dgraph-io/dgraph/...

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

**Dgraph**
Run the `test` script in the root folder.


    $ ./test
    
    Running tests. Ignoring vendor folder.
    ok      github.com/dgraph-io/dgraph/algo        0.013s
    ok      github.com/dgraph-io/dgraph/client      0.029s
    ok      github.com/dgraph-io/dgraph/client_test 2.841s
    …

**Badger**
Run `go test` in the root folder.


    $ go test ./...
    ok      github.com/dgraph-io/badger     24.853s
    ok      github.com/dgraph-io/badger/skl 0.027s
    ok      github.com/dgraph-io/badger/table       0.478s
    ok      github.com/dgraph-io/badger/y   0.004s
    
## Contributing

### Guidelines

Over years of writing big scalable systems, we are convinced that striving for simplicity wherever possible is the only way to build robust systems. This simplicity could be in design, could be in coding, or could be achieved by rewriting an entire module, that you may have painstakingly finished yesterday.


- **Pull requests are welcome**, as long as you're willing to put in the effort to meet the guidelines.
- Aim for clear, well written, maintainable code.
- Simple and minimal approach to features, like Go.
- Refactoring existing code now for better performance, better readability or better testability wins over adding a new feature.
- Don't add a function to a module that you don't use right now, or doesn't clearly enable a planned functionality.
- Don't ship a half done feature, which would significantl alterations to work fully.
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

Badger repo and files under `client/` and `x/` in Dgraph repo are licensed under the Apache license:


    /*
     * Copyright 2016-17 Dgraph Labs, Inc.
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

All other code in the Dgraph repo is licenses under the GNU AGPL v3 license:


    /*
     * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
     *
     * This program is free software: you can redistribute it and/or modify
     * it under the terms of the GNU Affero General Public License as published by
     * the Free Software Foundation, either version 3 of the License, or
     * (at your option) any later version.
     *
     * This program is distributed in the hope that it will be useful,
     * but WITHOUT ANY WARRANTY; without even the implied warranty of
     * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
     * GNU Affero General Public License for more details.
     *
     * You should have received a copy of the GNU Affero General Public License
     * along with this program.  If not, see <http://www.gnu.org/licenses/>.
     */

### Signed Commits

Signed commits help in verifying the authenticity of the contributor. We use signed commits in Dgraph, and we prefer it, though it's not compulsory to have signed commits. This is a recommended step for people who intend to contribute to Dgraph on a regular basis.

Follow instructions to generate and setup GPG keys for signing code commits on this [Github Help page](https://help.github.com/articles/signing-commits-with-gpg/).

