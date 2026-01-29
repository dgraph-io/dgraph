#
# SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
# SPDX-License-Identifier: Apache-2.0
#

BUILD          ?= $(shell git rev-parse --short HEAD)
BUILD_CODENAME ?= dgraph
BUILD_DATE     ?= $(shell git log -1 --format=%ci)
BUILD_BRANCH   ?= $(shell git rev-parse --abbrev-ref HEAD)
BUILD_VERSION  ?= $(shell git describe --always --tags)

export GOPATH  ?= $(shell go env GOPATH)
GOHOSTOS       := $(shell go env GOHOSTOS)
GOHOSTARCH     := $(shell go env GOHOSTARCH)

# On non-Linux systems, use a separate directory for Linux binaries
ifeq ($(GOHOSTOS),linux)
    export LINUX_GOBIN ?= $(GOPATH)/bin
else
    export LINUX_GOBIN ?= $(GOPATH)/linux_$(GOHOSTARCH)
endif

######################
# Build & Release Parameters
# DGRAPH_VERSION flag facilitates setting the dgraph version
# DGRAPH_VERSION flag is used for our release pipelines, where it is set to our release version number automatically
# DGRAPH_VERSION defaults to local, if not specified, for development purposes
######################
DGRAPH_VERSION ?= local

.PHONY: all
all: dgraph ## Build all targets

.PHONY: dgraph
dgraph: ## Build dgraph binary
	$(MAKE) -w -C $@ all

.PHONY: dgraph-coverage
dgraph-coverage:
	$(MAKE) -w -C dgraph test-coverage-binary

.PHONY: version
version: ## Show build version info
	@echo Dgraph: ${BUILD_VERSION}
	@echo Build: ${BUILD}
	@echo Codename: ${BUILD_CODENAME}
	@echo Build date: ${BUILD_DATE}
	@echo Branch: ${BUILD_BRANCH}
	@echo Go version: $(shell go version)

.PHONY: install
install: ## Install dgraph binary
	@echo "Installing dgraph ($(GOHOSTOS)/$(GOHOSTARCH))..."
	@$(MAKE) -C dgraph install
ifneq ($(GOHOSTOS),linux)
	@mkdir -p $(LINUX_GOBIN)
	@echo "Installing dgraph (linux/$(GOHOSTARCH))..."
	@GOOS=linux GOARCH=$(GOHOSTARCH) $(MAKE) -C dgraph dgraph
	@mv dgraph/dgraph $(LINUX_GOBIN)/dgraph
	@echo "Installed dgraph (linux/$(GOHOSTARCH)) to $(LINUX_GOBIN)/dgraph"
endif


.PHONY: uninstall
uninstall: ## Uninstall dgraph binary
	@echo "Uninstalling Dgraph ..."; \
		$(MAKE) -C dgraph uninstall; \

.PHONY: dgraph-installed
dgraph-installed:
	@if [ ! -f "$(GOPATH)/bin/dgraph" ] || [ ! -f "$(LINUX_GOBIN)/dgraph" ]; then \
		echo "Dgraph binary missing, running make install..."; \
		$(MAKE) install; \
	fi

.PHONY: test
test: dgraph-installed local-image ## Run tests (see 'make help' for options)
ifdef TAGS
	@echo "Running tests with tags: $(TAGS)"
	go test -v --tags="$(TAGS)" \
		$(if $(TEST),--run="$(TEST)") \
		$(if $(PKG),./$(PKG)/...,./...)
else ifdef FUZZ
	@echo "Discovering and running fuzz tests..."
ifdef PKG
	go test -v -fuzz=Fuzz -fuzztime=$(or $(FUZZTIME),300s) ./$(PKG)/...
else
	@grep -r "^func Fuzz" --include="*_test.go" -l . 2>/dev/null | \
		xargs -I{} dirname {} | sort -u | while read dir; do \
			echo "Fuzzing $$dir..."; \
			go test -v -fuzz=Fuzz -fuzztime=$(or $(FUZZTIME),300s) ./$$dir/...; \
		done
endif
else
	@echo "Running test suite: $(or $(SUITE),all)"
	$(MAKE) -C t test args="--suite=$(or $(SUITE),all) $(if $(PKG),--pkg=\"$(PKG)\") $(if $(TEST),--test=\"$(TEST)\")"
endif

.PHONY: test-unit
test-unit: ## Unit tests, no Docker (i.e. 'SUITE=unit make test')
	@SUITE=unit $(MAKE) test

.PHONY: test-core
test-core: ## Core tests (i.e. 'SUITE=core make test')
	@SUITE=core $(MAKE) test

.PHONY: test-integration
test-integration: ## Integration tests (i.e. 'TAGS=integration make test')
	@TAGS=integration $(MAKE) test

.PHONY: test-integration2
test-integration2: ## Integration2 tests via dgraphtest (i.e. 'TAGS=integration2 make test')
	@TAGS=integration2 $(MAKE) test

.PHONY: test-upgrade
test-upgrade: ## Upgrade tests (i.e. 'TAGS=upgrade make test')
	@TAGS=upgrade $(MAKE) test

.PHONY: test-systest
test-systest: ## System integration tests (i.e. 'SUITE=systest make test')
	@SUITE=systest $(MAKE) test

.PHONY: test-vector
test-vector: ## Vector search tests (i.e. 'SUITE=vector make test')
	@SUITE=vector $(MAKE) test

.PHONY: test-fuzz
test-fuzz: ## Fuzz tests, auto-discovers packages (i.e. 'FUZZ=1 make test')
	@FUZZ=1 $(MAKE) test

.PHONY: test-ldbc
test-ldbc: ## LDBC benchmark tests (i.e. 'SUITE=ldbc make test')
	@SUITE=ldbc $(MAKE) test

.PHONY: test-load
test-load: ## Heavy load tests (i.e. 'SUITE=load make test')
	@SUITE=load $(MAKE) test

.PHONY: test-benchmark
test-benchmark: ## Go benchmarks (i.e. 'go test -bench')
	go test -bench=. -benchmem $(if $(PKG),./$(PKG)/...,./...)

.PHONY: local-image
local-image: ## Build local Docker image (dgraph/dgraph:local)
	@echo building local docker image
	@GOOS=linux GOARCH=amd64 $(MAKE) dgraph
	@mkdir -p linux
	@mv ./dgraph/dgraph ./linux/dgraph
	@docker build -f contrib/Dockerfile -t dgraph/dgraph:local .
	@rm -r linux

.PHONY: image-local
image-local: local-image ## Alias for local-image

.PHONY: docker-image
docker-image: dgraph ## Build Docker image (dgraph/dgraph:$VERSION)
	@mkdir -p linux
	@cp ./dgraph/dgraph ./linux/dgraph
	docker build -f contrib/Dockerfile -t dgraph/dgraph:$(DGRAPH_VERSION) .

.PHONY: docker-image-standalone
docker-image-standalone: dgraph docker-image
	@mkdir -p linux
	@cp ./dgraph/dgraph ./linux/dgraph
	$(MAKE) -w -C contrib/standalone all DOCKER_TAG=$(DGRAPH_VERSION) DGRAPH_VERSION=$(DGRAPH_VERSION)

.PHONY: coverage-docker-image
coverage-docker-image: dgraph-coverage
	@mkdir -p linux
	@cp ./dgraph/dgraph ./linux/dgraph
	docker build -f contrib/Dockerfile -t dgraph/dgraph:$(DGRAPH_VERSION) .

# build and run dependencies for ubuntu linux
.PHONY: linux-dependency
linux-dependency:
	sudo apt-get update
	sudo apt-get -y upgrade
	sudo apt-get -y install ca-certificates
	sudo apt-get -y install curl
	sudo apt-get -y	install gnupg
	sudo apt-get -y install lsb-release
	sudo apt-get -y install build-essential
	sudo apt-get -y install protobuf-compiler

.PHONY: help
help: ## Show available targets and variables
	@echo "Usage: make [target] [VAR=value ...]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'
	@echo ""
	@echo "Variables that can be passed to 'test':"
	@echo "  SUITE     Select t/ runner suite (e.g., make test SUITE=systest)"
	@echo "  TAGS      Go build tags - bypasses t/ runner (e.g., make test TAGS=integration2)"
	@echo "  PKG       Limit to specific package (e.g., make test PKG=systest/export)"
	@echo "  TEST      Run specific test function (e.g., make test TEST=TestGQLSchema)"
	@echo "  FUZZ      Enable fuzz testing (e.g., make test FUZZ=1)"
	@echo "  FUZZTIME  Fuzz duration per package (e.g., make test FUZZ=1 FUZZTIME=60s)"
	@echo ""
	@printf "  Available SUITE values: "
	@grep -o 'allowed := \[\]string{[^}]*}' t/t.go 2>/dev/null | \
		sed 's/allowed := \[\]string{"\([^}]*\)"}/\1/' | \
		tr -d '"' | tr ',' ' ' || echo "all, unit, core, systest, vector, ldbc, load"
	@printf "  Available TAGS values:  "
	@grep -roh "//go:build [a-z0-9]*" --include="*_test.go" . 2>/dev/null | \
		awk '{print $$2}' | \
		grep -E '^(integration|integration2|upgrade)$$' | \
		sort -u | tr '\n' ' ' && echo ""
	@echo ""
	@echo "Examples:"
	@echo "  make test TAGS=integration2 PKG=systest/vector        # integration2 tests for vector"
	@echo "  make test TAGS=upgrade PKG=acl TEST=TestACL           # specific upgrade test"
	@echo "  make test FUZZ=1 PKG=dql FUZZTIME=30s                 # fuzz dql package for 30s"
	@echo "  make test SUITE=systest PKG=systest/backup/filesystem # systest for backup pkg"
	@echo "  make test-benchmark PKG=posting                       # benchmark posting package"
