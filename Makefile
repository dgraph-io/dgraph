#
# Copyright 2023 Dgraph Labs, Inc. and Contributors
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
BUILD_CODENAME  = dgraph
BUILD_DATE     ?= $(shell git log -1 --format=%ci)
BUILD_BRANCH   ?= $(shell git rev-parse --abbrev-ref HEAD)
BUILD_VERSION  ?= $(shell git describe --always --tags)

DOCKER_TAG ?= local

.PHONY: version
version:
	@echo Dgraph: ${BUILD_VERSION}
	@echo Build: ${BUILD}
	@echo Codename: ${BUILD_CODENAME}
	@echo Build date: ${BUILD_DATE}
	@echo Branch: ${BUILD_BRANCH}
	@echo Go version: $(shell cat .go-version)
	@echo Docker Tag: ${DOCKER_TAG}

.PHONY: dgraph
dgraph:
	$(MAKE) -w -C $@ all

.PHONY: docker-image
docker-image:
	docker build -f Dockerfile -t dgraph/dgraph:$(DOCKER_TAG) \
	--build-arg BUILD="$(BUILD)" \
	--build-arg BUILD_CODENAME="$(BUILD_CODENAME)" \
	--build-arg BUILD_DATE="$(BUILD_DATE)" \
	--build-arg BUILD_BRANCH="$(BUILD_BRANCH)" \
	--build-arg BUILD_VERSION="$(BUILD_VERSION)" .

