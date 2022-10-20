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

MODIFIED        = $(shell git diff-index --quiet HEAD || echo "-mod")

GOPATH         ?= $(shell go env GOPATH)

###############

.PHONY: dgraph all oss version install install_oss oss_install uninstall test help image image-local local-image
all: dgraph

dgraph:
	GOOS=linux GOARCH=amd64 $(MAKE) -w -C $@ all

oss:
	GOOS=linux GOARCH=amd64 $(MAKE) BUILD_TAGS=oss

version:
	@echo Dgraph ${BUILD_VERSION}
	@echo Build: ${BUILD}
	@echo Codename: ${BUILD_CODENAME}${MODIFIED}
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

test: image-local
	@mv dgraph/dgraph ${GOPATH}/bin
	@$(MAKE) -C t test

image:
	@GOOS=linux GOARCH=amd64 $(MAKE) dgraph
	@mkdir -p linux
	@mv ./dgraph/dgraph ./linux/dgraph
	@docker build -f contrib/Dockerfile -t dgraph/dgraph:$(subst /,-,${BUILD_BRANCH}) .
	@rm -r linux

image-local local-image:
	@GOOS=linux GOARCH=amd64 $(MAKE) dgraph
	@mkdir -p linux
	@cp ./dgraph/dgraph ./linux/dgraph
	@docker build -f contrib/Dockerfile -t dgraph/dgraph:local .
	@rm -r linux

image-latest latest-image:
	@GOOS=linux GOARCH=amd64 $(MAKE) dgraph
	@mkdir -p linux
	@cp ./dgraph/dgraph ./linux/dgraph
	@docker build -f contrib/Dockerfile -t dgraph/dgraph:latest .
	@rm -r linux

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
