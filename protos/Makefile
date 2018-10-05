#
#  Copyright 2018 Dgraph Labs, Inc.
#
#  This file is available under the Apache License, Version 2.0,
#  with the Commons Clause restriction.
#

PROTO_PATH := ${GOPATH}/src:.
PROTO_PATH := ${PROTO_PATH}:${GOPATH}/src/github.com/gogo/protobuf
PROTO_PATH := ${PROTO_PATH}:${GOPATH}/src/github.com/dgraph-io/dgo/protos

.PHONY: help
help:
	@echo Use \"make regenerate\" to proceed.

clean:
	@mkdir -p pb && rm -f pb/pb.pb.go

regenerate: clean
	@protoc \
		--proto_path=${PROTO_PATH} \
		--gogofaster_out=plugins=grpc,Mapi.proto=github.com/dgraph-io/dgo/protos/api:pb \
		pb.proto
	@echo Done.
