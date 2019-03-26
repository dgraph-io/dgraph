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

.PHONY: check
check:
	@./depcheck.sh && \
		(echo "Upgrading proto libraries before regenerating." ; \
		go get -u github.com/golang/protobuf/protoc-gen-go ; \
		go get -u github.com/gogo/protobuf/protoc-gen-gofast)

.PHONY: regenerate
regenerate: check clean
	@protoc \
		--proto_path=${PROTO_PATH} \
		--gofast_out=plugins=grpc,Mapi.proto=github.com/dgraph-io/dgo/protos/api:pb \
		pb.proto
	@echo Done.
