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
.PHONY: all build version

export DGRAPH_VERSION ?= latest

all: build version

build:
	./hooks/build

version:
	docker run dgraph/standalone:$(DGRAPH_VERSION) dgraph version
