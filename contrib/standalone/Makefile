# Build dgraph/standalone image
# Usage:
#
# For latest tag:
#     Build, based on dgraph/dgraph:latest:
#       make
#     Push:
#       make push
#
# For $TAG tag, set DOCKER_TAG:
#     Build, based on dgraph/dgraph:latest:
#       make DOCKER_TAG=$TAG
#     Push:
#       make DOCKER_TAG=$TAG push
#
# Examples
#     Build and push latest:
#       make
#       make push
#
#     Build and push master:
#       make DGRAPH_VERSION=master
#       make DGRAPH_VERSION=master push
#
.PHONY: all build

export DGRAPH_VERSION ?= local

all: build

build:
	./hooks/build
