#
# Copyright 2018 Dgraph Labs, Inc. and Contributors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

BUILD          ?= $(shell git rev-parse --short HEAD)
BUILD_CODENAME  = unnamed
BUILD_DATE     ?= $(shell git log -1 --format=%ci)
BUILD_BRANCH   ?= $(shell git rev-parse --abbrev-ref HEAD)
BUILD_VERSION  ?= $(shell git describe --always --tags)

GOPATH         ?= $(shell go env GOPATH)

######################
# Release Paramaters #
# by default will be set to local

# make docker-image DOCKER_TAG=22.0.0
DOCKER_TAG     ?= local
# make dgraph DGRAPH_RELEASE_VERSION=22.0.0
DGRAPH_RELEASE_VERSION ?= local

######################

.PHONY: dgraph all oss version install install_oss oss_install uninstall test help image image-local local-image
all: dgraph

dgraph:
	GOOS=linux GOARCH=amd64 $(MAKE) -w -C $@ all

oss:
	GOOS=linux GOARCH=amd64 $(MAKE) BUILD_TAGS=oss

version:
	@echo Dgraph ${BUILD_VERSION}
	@echo Build: ${BUILD}
	@echo Codename: ${BUILD_CODENAME}
	@echo Build date: ${BUILD_DATE}
	@echo Branch: ${BUILD_BRANCH}
	@echo Go version: $(shell go version)

install:
	@echo "Installing dgraph ..."; \
		GOOS=linux GOARCH=amd64 $(MAKE) -C dgraph install; \

install_oss oss_install:
	GOOS=linux GOARCH=amd64 $(MAKE) BUILD_TAGS=oss install

uninstall:
	@echo "Uninstalling dgraph ..."; \
		$(MAKE) -C dgraph uninstall; \

test: dgraph
	@$(MAKE) -C t test

docker-image: dgraph
	docker build -f contrib/Dockerfile -t dgraph/dgraph:$(DOCKER_TAG) .

docker-image-standalone: dgraph
	$(MAKE) -w -C contrib/standalone all DOCKER_TAG=$(DOCKER_TAG) DGRAPH_VERSION=$(DGRAPH_RELEASE_VERSION)

# build and run dependencies for ubuntu linux
dependency:
	sudo apt-get update
	sudo apt-get -y upgrade
	sudo apt-get -y install ca-certificates
	sudo apt-get -y install curl
	sudo apt-get -y	install gnupg
	sudo apt-get -y install lsb-release
	sudo apt-get -y install build-essential
	sudo apt-get -y install protobuf-compiler

help:
	@echo
	@echo Build commands:
	@echo "  make [all]     - Build all targets [EE]"
	@echo "  make oss       - Build all targets [OSS]"
	@echo "  make dgraph    - Build dgraph binary"
	@echo "  make install   - Install all targets"
	@echo "  make uninstall - Uninstall known targets"
	@echo "  make version   - Show current build info"
	@echo "  make help      - This help"
	@echo "  make test      - Make local image and run t.go"
	@echo
