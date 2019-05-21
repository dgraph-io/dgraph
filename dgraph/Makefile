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

BIN            = dgraph
BUILD         ?= $(shell git rev-parse --short HEAD)
BUILD_DATE    ?= $(shell git log -1 --format=%ci)
BUILD_BRANCH  ?= $(shell git rev-parse --abbrev-ref HEAD)
BUILD_VERSION ?= $(shell git describe --always --tags)
BUILD_TAGS    ?=
GOPATH        ?= $(shell go env GOPATH)

# Build-time Go variables
dgraphVersion  = github.com/dgraph-io/dgraph/x.dgraphVersion
gitBranch      = github.com/dgraph-io/dgraph/x.gitBranch
lastCommitSHA  = github.com/dgraph-io/dgraph/x.lastCommitSHA
lastCommitTime = github.com/dgraph-io/dgraph/x.lastCommitTime

BUILD_FLAGS   ?= -ldflags '-X ${lastCommitSHA}=${BUILD} -X "${lastCommitTime}=${BUILD_DATE}" -X "${dgraphVersion}=${BUILD_VERSION}" -X ${gitBranch}=${BUILD_BRANCH}'

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

.PHONY: all $(BIN)
all: $(BIN)

$(BIN): clean
	@go build $(BUILD_FLAGS) -o $(BIN)

clean:
	@rm -f $(BIN)

uninstall:
	@go clean -i -x

install:
	@echo "Old SHA256:" `sha256sum $(GOPATH)/bin/$(BIN) 2>/dev/null | cut -c-64`
	@go install $(BUILD_FLAGS)
	@echo "New SHA256:" `sha256sum $(GOPATH)/bin/$(BIN) 2>/dev/null | cut -c-64`
