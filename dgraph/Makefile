#
#  Copyright 2018 Dgraph Labs, Inc.
#
#  This file is available under the Apache License, Version 2.0,
#  with the Commons Clause restriction.
#

BIN            = dgraph
BUILD         ?= $(shell git rev-parse --short HEAD)
BUILD_DATE    ?= $(shell git log -1 --format=%ci)
BUILD_BRANCH  ?= $(shell git rev-parse --abbrev-ref HEAD)
BUILD_VERSION ?= $(shell git describe --always --tags)

# Build-time Go variables
dgraphVersion  = github.com/dgraph-io/dgraph/x.dgraphVersion
gitBranch      = github.com/dgraph-io/dgraph/x.gitBranch
lastCommitSHA  = github.com/dgraph-io/dgraph/x.lastCommitSHA
lastCommitTime = github.com/dgraph-io/dgraph/x.lastCommitTime

.PHONY: all $(BIN)
all: $(BIN)

$(BIN): clean
	@go build -ldflags '-X ${lastCommitSHA}=${BUILD} -X "${lastCommitTime}=${BUILD_DATE}" -X ${dgraphVersion}=${BUILD_VERSION} -X ${gitBranch}=${BUILD_BRANCH}' -o $(BIN)

clean:
	@rm -f $(BIN)

uninstall:
	@go clean -i -x

install:
	@go install
