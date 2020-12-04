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

DGO_PATH    := github.com/dgraph-io/dgo/v200
BADGER_PATH := github.com/dgraph-io/badger/v2
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

.PHONY: check
check:
	@./depcheck.sh && \
		(echo "Installing proto libraries to versions in go.mod." ; \
		go install github.com/golang/protobuf/protoc-gen-go ; \
		go install github.com/gogo/protobuf/protoc-gen-gofast)

.PHONY: copy-protos
copy-protos:
	@mkdir -p ${TMPDIR}/src/${DGO_PATH}/protos
	@mkdir -p ${TMPDIR}/src/${BADGER_PATH}/pb
	@mkdir -p ${TMPDIR}/src/${GOGO_PATH}/gogoproto
	@cp $(shell go list -m -f "{{.Dir}}" ${BADGER_PATH})/pb/badgerpb2.proto ${TMPDIR}/src/${BADGER_PATH}/pb/pb.proto
	@cp $(shell go list -m -f "{{.Dir}}" ${DGO_PATH})/protos/api.proto ${TMPDIR}/src/${DGO_PATH}/protos/api.proto
	@cp $(shell go list -m -f "{{.Dir}}" ${GOGO_PATH})/gogoproto/gogo.proto ${TMPDIR}/src/${GOGO_PATH}/gogoproto/gogo.proto

.PHONY: regenerate
regenerate: copy-protos check clean
	@protoc \
		--proto_path=/usr/local/include \
		--proto_path=/usr/include \
		--proto_path=${PROTO_PATH} \
		--gofast_out=plugins=grpc,Mapi.proto=${DGO_PATH}/protos/api:pb \
		pb.proto
	@rm -rf ${TMPDIR}
	@echo Done.
