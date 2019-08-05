#!/usr/bin/env bash

set -e

readonly PROTOCMINVER="3.6.1"
readonly PROTOGOREQVER="v1.3.2"
readonly PROTOGOGOREQVER="v1.2.1"

which protoc &>/dev/null || (echo "Error: protoc not found" ; exit 1)


PROTOCVER=`protoc --version | awk '{printf $2}'`
PROTOGOVER=`git -C $GOPATH/src/github.com/golang/protobuf describe --always --tags`
PROTOGOGOVER=`git -C $GOPATH/src/github.com/gogo/protobuf describe --always --tags`

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

CompareSemVer $PROTOCMINVER $PROTOCVER "protoc"
CompareVer $PROTOGOREQVER $PROTOGOVER "golang/protobuf"
CompareVer $PROTOGOGOREQVER $PROTOGOGOVER "gogo/protobuf"

echo "OK"

exit 0
