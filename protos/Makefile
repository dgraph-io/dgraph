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

# Update BADGER_PB_VERSION when upgrading Badger major versions
BADGER_PB_VERSION = 3

DGO_PATH    := github.com/dgraph-io/dgo/v210
BADGER_PATH := github.com/dgraph-io/badger/v${BADGER_PB_VERSION}
GOGO_PATH   := github.com/gogo/protobuf

TMPDIR     := $(shell mktemp -d)
PROTO_PATH := ${TMPDIR}/src:.
PROTO_PATH := ${PROTO_PATH}:${TMPDIR}/src/${DGO_PATH}/protos
PROTO_PATH := ${PROTO_PATH}:${TMPDIR}/src/${BADGER_PATH}/pb

.PHONY: help
help:
	@echo Use \"make regenerate\" to proceed.

.PHONY: clean
clean:
	@mkdir -p pb && rm -f pb/pb.pb.go

.PHONY: tidy-deps
tidy-deps:
	@go mod tidy -v

.PHONY: check
check:
	@./depcheck.sh && \
		(echo "Installing proto libraries to versions in go.mod." ; \
		go install github.com/golang/protobuf/protoc-gen-go ; \
		go install github.com/gogo/protobuf/protoc-gen-gogofaster)

.PHONY: copy-protos
copy-protos:
	@mkdir -p ${TMPDIR}/src/${DGO_PATH}/protos
	@mkdir -p ${TMPDIR}/src/${BADGER_PATH}/pb
	@mkdir -p ${TMPDIR}/src/${GOGO_PATH}/gogoproto
	@cp $(shell go list -m -f "{{.Dir}}" ${BADGER_PATH})/pb/badgerpb${BADGER_PB_VERSION}.proto \
	 ${TMPDIR}/src/${BADGER_PATH}/pb/pb.proto
	@cp $(shell go list -m -f "{{.Dir}}" ${DGO_PATH})/protos/api.proto \
	 ${TMPDIR}/src/${DGO_PATH}/protos/api.proto
	@cp $(shell go list -m -f "{{.Dir}}" ${GOGO_PATH})/gogoproto/gogo.proto \
	 ${TMPDIR}/src/${GOGO_PATH}/gogoproto/gogo.proto

.PHONY: regenerate
regenerate: tidy-deps copy-protos check clean
	@protoc \
		--proto_path=/usr/local/include \
		--proto_path=/usr/include \
		--proto_path=${PROTO_PATH} \
		--gogofaster_out=plugins=grpc,Mapi.proto=${DGO_PATH}/protos/api:pb \
		pb.proto
	@rm -rf ${TMPDIR}
	@echo Done.
