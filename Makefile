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
all: dgraph

.PHONY: dgraph
dgraph:
	$(MAKE) -w -C $@ all

.PHONY: dgraph-coverage
dgraph-coverage:
	$(MAKE) -w -C dgraph test-coverage-binary

.PHONY: version
version:
	@echo Dgraph: ${BUILD_VERSION}
	@echo Build: ${BUILD}
	@echo Codename: ${BUILD_CODENAME}
	@echo Build date: ${BUILD_DATE}
	@echo Branch: ${BUILD_BRANCH}
	@echo Go version: $(shell go version)

.PHONY: install
install:
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
uninstall:
	@echo "Uninstalling Dgraph ..."; \
		$(MAKE) -C dgraph uninstall; \

.PHONY: dgraph-installed
dgraph-installed:
	@if [ ! -f "$(GOPATH)/bin/dgraph" ] || [ ! -f "$(LINUX_GOBIN)/dgraph" ]; then \
		echo "Dgraph binary missing, running make install..."; \
		$(MAKE) install; \
	fi

.PHONY: test
test: dgraph-installed local-image
	@$(MAKE) -C t test

.PHONY: image-local local-image
image-local local-image:
	@echo building local docker image
	@GOOS=linux GOARCH=amd64 $(MAKE) dgraph
	@mkdir -p linux
	@mv ./dgraph/dgraph ./linux/dgraph
	@docker build -f contrib/Dockerfile -t dgraph/dgraph:local .
	@rm -r linux

.PHONY: docker-image
docker-image: dgraph
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
help:
	@echo
	@echo Build commands:
	@echo "  make [all]     - Build all targets [EE]"
	@echo "  make dgraph    - Build dgraph binary"
	@echo "  make install   - Install all targets"
	@echo "  make uninstall - Uninstall known targets"
	@echo "  make version   - Show current build info"
	@echo "  make help      - This help"
	@echo "  make test      - Make local image and run t.go"
	@echo
