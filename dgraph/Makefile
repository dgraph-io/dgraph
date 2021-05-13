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

USER_ID         = $(shell id -u)
BIN             = dgraph
BUILD          ?= $(shell git rev-parse --short HEAD)
BUILD_CODENAME  = unnamed
BUILD_DATE     ?= $(shell git log -1 --format=%ci)
BUILD_BRANCH   ?= $(shell git rev-parse --abbrev-ref HEAD)

# Use the next build version on master branch
# NOTE: this needs to be updated with each new release
ifeq ($(BUILD_BRANCH),master)
# Version on master is of the format <next-release-version>-g<commit-id>
BUILD_VERSION ?= 'v21.07.0-g'$(BUILD)
else
BUILD_VERSION ?= $(shell git describe --always --tags)
endif

GOOS          ?= $(shell go env GOOS)
# Only build with jemalloc on Linux, mac
ifeq ($(GOOS),$(filter $(GOOS),linux darwin))
	BUILD_TAGS ?= jemalloc
endif
GOPATH        ?= $(shell go env GOPATH)
MODIFIED = $(shell git diff-index --quiet HEAD || echo "-mod")

export GO111MODULE := on

# Build-time Go variables
dgraphVersion   = github.com/dgraph-io/dgraph/x.dgraphVersion
dgraphCodename  = github.com/dgraph-io/dgraph/x.dgraphCodename
gitBranch       = github.com/dgraph-io/dgraph/x.gitBranch
lastCommitSHA   = github.com/dgraph-io/dgraph/x.lastCommitSHA
lastCommitTime  = github.com/dgraph-io/dgraph/x.lastCommitTime

BUILD_FLAGS   ?= -ldflags '-X ${lastCommitSHA}=${BUILD} -X "${lastCommitTime}=${BUILD_DATE}" -X "${dgraphVersion}=${BUILD_VERSION}" -X "${dgraphCodename}=${BUILD_CODENAME}${MODIFIED}" -X ${gitBranch}=${BUILD_BRANCH}'

# Insert build tags if specified
ifneq ($(strip $(BUILD_TAGS)),)
	BUILD_FLAGS += -tags '$(BUILD_TAGS)'
	ifneq (,$(findstring oss,$(BUILD_TAGS)))
		BUILD_VERSION := $(BUILD_VERSION)-oss
	endif
endif

# Build with compiler optimizations disabled, which will help debugging with dlv.
ifneq ($(strip $(BUILD_DEBUG)),)
	BUILD_FLAGS += -gcflags="all=-N -l"
endif

# Build with race detector enabled.
ifneq ($(strip $(BUILD_RACE)),)
	BUILD_FLAGS += -race
endif

# jemalloc stuff
HAS_JEMALLOC = $(shell test -f /usr/local/lib/libjemalloc.a && echo "jemalloc")
JEMALLOC_URL = "https://github.com/jemalloc/jemalloc/releases/download/5.2.1/jemalloc-5.2.1.tar.bz2"

.PHONY: all $(BIN)
all: $(BIN)

$(BIN): clean jemalloc
	@go build $(BUILD_FLAGS) -o $(BIN)

clean:
	@rm -f $(BIN)

uninstall:
	@go clean -i -x

install: jemalloc
	@echo "Commit SHA256: `git rev-parse HEAD`"
	@echo "Old SHA256:" `sha256sum $(GOPATH)/bin/$(BIN) 2>/dev/null | cut -c-64`
	@go install $(BUILD_FLAGS)
	@echo "New SHA256:" `sha256sum $(GOPATH)/bin/$(BIN) 2>/dev/null | cut -c-64`

jemalloc:
	@if [ -z "$(HAS_JEMALLOC)" ] ; then \
		mkdir -p /tmp/jemalloc-temp && cd /tmp/jemalloc-temp ; \
		echo "Downloading jemalloc" ; \
		curl -s -L ${JEMALLOC_URL} -o jemalloc.tar.bz2 ; \
		tar xjf ./jemalloc.tar.bz2 ; \
		cd jemalloc-5.2.1 ; \
		./configure --with-jemalloc-prefix='je_' --with-malloc-conf='background_thread:true,metadata_thp:auto'; \
		make ; \
		if [ "$(USER_ID)" = "0" ]; then \
			make install ; \
		else \
			echo "==== Need sudo access to install jemalloc" ; \
			sudo make install ; \
		fi \
	fi

