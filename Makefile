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
test-unit: ## Run unit tests (no Docker required)
	@SUITE=unit $(MAKE) test

.PHONY: test-core
test-core: ## Run core tests
	@SUITE=core $(MAKE) test

.PHONY: test-integration
test-integration: ## Run integration tests (go test with tags)
	@TAGS=integration $(MAKE) test

.PHONY: test-integration2
test-integration2: ## Run integration2 tests via dgraphtest
	@TAGS=integration2 $(MAKE) test

.PHONY: test-upgrade
test-upgrade: ## Run upgrade tests
	@TAGS=upgrade $(MAKE) test

.PHONY: test-systest
test-systest: ## Run system integration tests
	@SUITE=systest $(MAKE) test

.PHONY: test-vector
test-vector: ## Run vector search tests
	@SUITE=vector $(MAKE) test

.PHONY: test-fuzz
test-fuzz: ## Run fuzz tests (auto-discovers packages)
	@FUZZ=1 $(MAKE) test

.PHONY: test-ldbc
test-ldbc: ## Run LDBC benchmark tests
	@SUITE=ldbc $(MAKE) test

.PHONY: test-load
test-load: ## Run heavy load tests
	@SUITE=load $(MAKE) test

.PHONY: test-benchmark
test-benchmark: ## Run Go benchmarks
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
	@echo "  SUITE     Select t/ runner suite (e.g., SUITE=systest make test)"
	@echo "  TAGS      Go build tags - bypasses t/ runner (e.g., TAGS=integration2 make test)"
	@echo "  PKG       Limit to specific package (e.g., PKG=systest/export make test)"
	@echo "  TEST      Run specific test function (e.g., TEST=TestGQLSchema make test)"
	@echo "  FUZZ      Enable fuzz testing (e.g., FUZZ=1 make test)"
	@echo "  FUZZTIME  Fuzz duration per package (e.g., FUZZ=1 FUZZTIME=60s make test)"
	@echo ""
	@echo "Examples:"
	@echo "  TAGS=integration2 PKG=systest/vector make test        # integration2 tests for vector"
	@echo "  TAGS=upgrade PKG=acl TEST=TestACL make test           # specific upgrade test"
	@echo "  FUZZ=1 PKG=dql FUZZTIME=30s make test                 # fuzz dql package for 30s"
	@echo "  SUITE=systest PKG=systest/backup/filesystem make test # systest for backup pkg"
