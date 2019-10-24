#!/bin/bash


gogo_protobuf_version=$(awk '/github.com\/gogo\/protobuf/ { print $2 }' $(go env GOMOD))
golang_protobuf_version=$(awk '/github.com\/golang\/protobuf/ { print $2 }' $(go env GOMOD))

gogo_protobuf_expected_version='v1.2.0'
golang_protobuf_expected_version='v1.3.2'

if [ $gogo_protobuf_version != $gogo_protobuf_expected_version ]; then
    printf 'github.com/gogo/protobuf (%s) does not match expected version (%s).\n' $gogo_protobuf_version $gogo_protobuf_expected_version
    exit 1
fi

if [ $golang_protobuf_version != $golang_protobuf_expected_version ]; then
    printf 'github.com/golang/protobuf (%s) does not match expected version (%s).\n' $golang_protobuf_version $golang_protobuf_expected_version
    exit 1
fi

outputproto=pb.proto

tmpdir="$(mktemp -d -t regen-protos.XXXXXX)" trap 'rm -rf $tmpdir' EXIT

dgo=$(awk '/github.com\/dgraph-io\/dgo/ { printf("%s@%s", $1,$2);}' $(go env
GOMOD)) badger=$(awk '/github.com\/dgraph-io\/badger/ { printf("%s@%s",
$1,$2);}' $(go env GOMOD))

GOMOD_PATH="$(go env GOPATH)/pkg/mod"

# https://github.com/golang/protobuf#parameters Import paths for dgo and
# badger packages. Paths must be updated when semantic versions are released
# e.g., v2. For Badger's proto path, we include the directory prefix pb/ to
# disambiguate Dgraph's pb.proto from Badger's pb.proto. #
# DGO_PROTO="api.proto" #
# DGO_IMPORT_PATH="github.com/dgraph-io/dgo/v2/protos/api" #
# BADGER_PROTO="pb/pb.proto" #
# BADGER_IMPORT_PATH="github.com/dgraph-io/badger/pb"

PROTO_PATH=. PROTO_PATH=${PROTO_PATH}:${GOMOD_PATH}/${dgo}/protos
PROTO_PATH=${PROTO_PATH}:${GOMOD_PATH}/${badger}

mkdir -p $tmpdir/bin PATH=$tmpdir/bin:$PATH GOBIN=$tmpdir/bin go get -u
github.com/golang/protobuf/protoc-gen-go@$protoversion GOBIN=$tmpdir/bin go
get -u github.com/gogo/protobuf/protoc-gen-gofast@$protoversion

protoc \ --proto_path="${PROTO_PATH}" \
--gofast_out=plugins=grpc,Mapi.proto=github.com/dgraph-io/dgo/v2/protos/api:pb
\ $outputproto

protoc \ --proto_path=${PROTO_PATH} \
--gofast_out="plugins=grpc,M${DGO_PROTO}=${DGO_IMPORT_PATH},M${BADGER_PROTO}=${BADGER_IMPORT_PATH}:pb"
\ $outputproto

echo "Done"
