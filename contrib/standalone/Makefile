# Build dgraph/standalone image
# Usage:
#
# For latest tag:
#     Build, based on dgraph/dgraph:latest:
#       make
#     Push:
#       make push
#
# For $TAG tag, set DGRAPH_VERSION:
#     Build, based on dgraph/dgraph:latest:
#       make DGRAPH_VERSION=$TAG
#     Push:
#       make DGRAPH_VERSION=$TAG push
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
.PHONY: all build push version

export DGRAPH_VERSION ?= latest

all: build version

build:
	./hooks/build

push:
	docker push dgraph/standalone:$(DGRAPH_VERSION)

version:
	docker run dgraph/standalone:$(DGRAPH_VERSION) dgraph version
