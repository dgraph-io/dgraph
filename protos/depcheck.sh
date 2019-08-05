#!/usr/bin/env bash

set -e

# Required minimum version for protoc protocol compiler
readonly PROTOC_MINVER="3.6.1"
# Required version for golang/protobuf/protoc-gen-go
readonly GOLANG_PROTOBUF_REQVER="v1.3.2"
# Required version for gogo/protobuf/protoc-gen-gofast
readonly GOGO_PROTOBUF_REQVER="v1.2.1"

which protoc &>/dev/null || (echo "Error: protoc not found" ; exit 1)

PROTOC_VER=`protoc --version | awk '{printf $2}'`
GOLANG_PROTOBUF_VER=`git -C $GOPATH/src/github.com/golang/protobuf describe --always --tags`
GOGO_PROTOBUF_VER=`git -C $GOPATH/src/github.com/gogo/protobuf describe --always --tags`

# CompareSemVer compares the minimum version minver against another version curver.
# If the version is below our min it will exit with non-zero to trigger error in make.
function CompareSemVer() {
	local minver=(${1//./ })
	local curver=(${2//./ })
        local name="$3"

	echo "Checking for semantic version $1 or newer for $3"

	for i in 0 1 2; do
		if [ ${minver[$i]} -gt ${curver[$i]} ]; then
			echo "Error: version $2 is lower than the required version $1"
			exit 1
		elif [ ${curver[$i]} -gt ${minver[$i]} ]; then
			break
		fi
	done
}

# CompareVer compares the required version reqver against another version curver.
# If the versions do not match it will exit with non-zero to trigger error in make.
function CompareVer() {
    local reqver="$1"
    local curver="$2"
    local name="$3"

    echo "Checking for exact version $reqver for $name"
    if [ "$reqver" != "$curver" ]; then
        echo "Error: $name version $curver is not the required version $reqver"
        exit 1
    fi
}

CompareSemVer $PROTOC_MINVER $PROTOC_VER "protoc"
CompareVer $GOLANG_PROTOBUF_REQVER $GOLANG_PROTOBUF_VER "golang/protobuf"
GOGO_PROTOBUF_VER $GOGO_PROTOBUF_REQVER $GOGO_PROTOBUF_VER "gogo/protobuf"

echo "OK"

exit 0
