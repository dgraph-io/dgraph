#
# Copyright 2025 Hypermode Inc. and Contributors
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

# linux || darwin
GOOS          ?= $(shell go env GOOS)
GOPATH        ?= $(shell go env GOPATH)

.PHONY: test

all: test

test: 
# 	build the t.go binary
	@go build .
#	clean go testcache
	@go clean -testcache
#	run t.go with specified arguments; otherwise run standard suite
	@if [ -n "$(args)" ]; then \
		./t $(args); \
	else \
		./t; \
	fi
#	clean up docker containers after test execution
	@./t -r
