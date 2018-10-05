#
#  Copyright 2018 Dgraph Labs, Inc.
#
#  This file is available under the Apache License, Version 2.0,
#  with the Commons Clause restriction.
#

BUILD         ?= $(shell git rev-parse --short HEAD)
BUILD_DATE    ?= $(shell git log -1 --format=%ci)
BUILD_BRANCH  ?= $(shell git rev-parse --abbrev-ref HEAD)
BUILD_VERSION ?= $(shell git describe --always --tags)

SUBDIRS = dgraph

###############

.PHONY: all $(SUBDIRS)
all: $(SUBDIRS)

$(SUBDIRS):
	$(MAKE) -w -C $@ all

deps:
	@echo Synching dependencies...
	@go get github.com/kardianos/govendor
	@govendor sync
	@echo Done.

version:
	@echo Dgraph ${BUILD_VERSION}
	@echo Build: ${BUILD}
	@echo Build date: ${BUILD_DATE}
	@echo Branch: ${BUILD_BRANCH}

help:
	@echo
	@echo Build commands:
	@echo "  make [all]   - Build all targets"
	@echo "  make dgraph  - Build only dgraph binary"
	@echo "  make deps    - Sync vendor deps"
	@echo "  make version - Show current build info"
	@echo "  make help    - This help"
	@echo
