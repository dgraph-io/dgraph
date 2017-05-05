#!/bin/bash
# This file loops through all the proto files under protos folder, checks if they
# have changed, compiles them if they have and generates the relevant .pb.go files.


hasChanged() {
	local path=$1
	if [[ -n $(git diff $path) ]] || [[ -n $(git diff --staged $path) ]];then
		return 0;
	fi
	return 1;
}

protos=$GOPATH/src/github.com/dgraph-io/dgraph/protos
cd $protos
for f in $protos/*.proto;
do
	path=$(realpath --relative-to=$protos $f)
	echo $path
	#if hasChanged $path; then
		protoc --gofast_out=plugins=grpc:$GOPATH/src --proto_path=. $path
	#fi
done
