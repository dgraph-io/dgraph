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

PROTOC_VERSION = "3.6.1"
GOLANG_PROTOBUF_VERSION := "v1.3.2"
GOGO_PROTOBUF_VERSION = "v1.2.1"
GOBIN := $(GOPATH)/bin
PROTO_PATH := ${GOPATH}/src:.
PROTO_PATH := ${PROTO_PATH}:${GOPATH}/src/github.com/dgraph-io/dgraph
PROTO_PATH := ${PROTO_PATH}:${GOPATH}/src/github.com/dgraph-io/dgo/protos
PROTO_PATH := ${PROTO_PATH}:${GOPATH}/src/github.com/dgraph-io/badger/pb

.PHONY: help
help:
	@echo Use \"make regenerate\" to proceed.

.PHONY: clean
clean:
	@mkdir -p pb && rm -f pb/pb.pb.go
	@rm -f $(GOBIN)/protoc-gen-go
	@rm -f $(GOBIN)/protoc-gen-gofast

.PHONY: check
check:
	@./depcheck.sh

.PHONY: golang_protobuf
golang_protobuf:
	@rm -rf $(GOPATH)/src/github.com/golang/protobuf
	@echo "Cloning  $(GOPATH)/src/github.com/golang/protobuf to $(GOLANG_PROTOBUF_VERSION)"
	@git clone --branch $(GOLANG_PROTOBUF_VERSION) --single-branch --depth 1 https://github.com/golang/protobuf $(GOPATH)/src/github.com/golang/protobuf
	@go install $(GOPATH)/src/github.com/golang/protobuf/protoc-gen-go

.PHONY: gogo_protobuf
gogo_protobuf:
	@rm -rf $(GOPATH)/src/github.com/gogo/protobuf
	@echo "Cloning  $(GOPATH)/src/github.com/gogo/protobuf to $(GOGO_PROTOBUF_VERSION)"
	@git clone --branch $(GOGO_PROTOBUF_VERSION) --single-branch --depth 1 https://github.com/gogo/protobuf $(GOPATH)/src/github.com/gogo/protobuf
	@go install $(GOPATH)/src/github.com/gogo/protobuf/protoc-gen-gofast

.PHONY: regenerate
regenerate: check clean golang_protobuf gogo_protobuf
	@protoc \
		--proto_path=${PROTO_PATH} \
		--gofast_out=plugins=grpc,Mapi.proto=github.com/dgraph-io/dgo/protos/api:pb \
		pb.proto
	@echo Done.
