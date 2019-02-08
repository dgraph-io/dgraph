#!/usr/bin/env bash

set -e

readonly PROTOCMINVER="3.6.1"

which protoc &>/dev/null || (echo "Error: protoc not found" ; exit 1)

PROTOCVER=`protoc --version | awk '{printf $2}'`

# CompareSemVer compares the minimum version minver against another version curver.
# If the version is below our min it will exit with non-zero to trigger error in make.
function CompareSemVer() {
	local minver=(${1//./ })
	local curver=(${2//./ })

	echo "Checking for semantic version $1 or newer"

	for i in 0 1 2; do
		if [ ${minver[$i]} -gt ${curver[$i]} ]; then
			echo "Error: version $2 is lower than the required version $1"
			exit 1
		elif [ ${curver[$i]} -gt ${minver[$i]} ]; then
			break
		fi
	done
}

CompareSemVer $PROTOCMINVER $PROTOCVER

# TODO: check proto api versions

echo "OK"

exit 0
