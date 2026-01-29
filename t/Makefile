#
# SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
# SPDX-License-Identifier: Apache-2.0
#

# linux || darwin
GOOS          ?= $(shell go env GOOS)
export GOPATH ?= $(shell go env GOPATH)
GOHOSTOS      := $(shell go env GOHOSTOS)
GOHOSTARCH    := $(shell go env GOHOSTARCH)

# On non-Linux systems, use a separate directory for Linux binaries
ifeq ($(GOHOSTOS),linux)
    export GOPATH_LINUX_BIN ?= $(GOPATH)/bin
else
    export GOPATH_LINUX_BIN ?= $(GOPATH)/linux_$(GOHOSTARCH)
endif

all: test

.PHONY: check
check: check-go check-jq check-docker check-gotestsum check-ack
	@if [ "$(GOOS)" = "linux" ]; then \
		which protoc > /dev/null 2>&1 || (echo "Error: protoc is not installed or not in PATH" && exit 1); \
	fi
	@echo "All dependencies are installed"
	@echo "GOPATH_LINUX_BIN=$(GOPATH_LINUX_BIN)"
	@if [ -f "$(GOPATH_LINUX_BIN)/dgraph" ]; then \
		file $(GOPATH_LINUX_BIN)/dgraph | grep -q "ELF.*executable" || (echo "Error: dgraph binary at $(GOPATH_LINUX_BIN)/dgraph is not a Linux executable" && exit 1); \
	else \
		echo "Error: dgraph binary not found at $(GOPATH_LINUX_BIN)/dgraph" && exit 1; \
	fi
	@echo "The dgraph binary is a Linux executable (as required)"

.PHONY: check-jq
check-jq:
	@./scripts/check-jq.sh

.PHONY: check-docker
check-docker:
	@./scripts/check-docker.sh

.PHONY: check-gotestsum
check-gotestsum:
	@./scripts/check-gotestsum.sh

.PHONY: check-ack
check-ack:
	@./scripts/check-ack.sh

.PHONY: check-protoc
check-protoc:
	@./scripts/check-protoc.sh

.PHONY: check-go
check-go:
	@./scripts/check-go.sh

.PHONY: check-brew
check-brew:
	@./scripts/check-brew.sh

.PHONY: dgraph-installed
dgraph-installed:
	@$(MAKE) -C .. dgraph-installed

.PHONY: test
test: dgraph-installed check
# 	build the t.go binary
	@go build .
#	clean go testcache
	@go clean -testcache
#	run t.go with specified arguments; otherwise run standard suite
	@if [ -n "$(args)" ]; then \
		./t $(args); \
	else \
		./t; \
	fi
#	clean up docker containers after test execution
	@./t -r
