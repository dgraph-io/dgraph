# Build dgraph/standalone image
# Usage:
#  Build latest tag, based on dgraph/dgraph:latest
#    make
#
#  Build $TAG tag, based on dgraph/dgraph:$TAG
#    make DGRAPH_VERSION=$TAG
#
# Examples
#  Build master:
#    make DGRAPH_VERSION=master
.PHONY: all build push version

export DOCKER_IMAGE ?= dgraph/standalone
export DGRAPH_VERSION ?= latest

all: build version

build:
	./hooks/build

push:
	docker push $(DOCKER_IMAGE):$(DGRAPH_VERSION)

version:
	docker run $(DOCKER_IMAGE):$(DGRAPH_VERSION) dgraph version
