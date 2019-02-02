#!/usr/bin/env bash

set -e

readonly PROTOCMINVER="3.6.1"

which protoc &>/dev/null || (echo "Error: protoc not found" ; exit 1)

PROTOCVER=`protoc --version | awk '{printf $2}'`

# CompareSemVer compares the minimum version ver1 against another version ver2.
# If the version is below our min it will exit with non-zero to trigger error in make.
function CompareSemVer() {
	local ver1=(${1//./ })
	local ver2=(${2//./ })

	echo "Checking for semver $1 or newer"

	# check major
	if [ ${ver1[0]} -gt ${ver2[0]} ]; then
		echo "Error: major version is '${ver2[0]}'"
		exit 1
	elif [ ${ver2[0]} -gt ${ver1[0]} ]; then
		exit 0
	fi

	# check minor
	if [ ${ver1[1]} -gt ${ver2[1]} ]; then
		echo "Error: minor version is '${ver2[1]}'"
		exit 1
	elif [ ${ver2[1]} -gt ${ver1[1]} ]; then
		exit 0
	fi

	# check patch
	if [ ${ver1[2]} -gt ${ver2[2]} ]; then
		echo "Error patch version is '${ver2[2]}'"
		exit 1
	fi
}

CompareSemVer $PROTOCMINVER $PROTOCVER

# TODO: check proto api versions

echo "OK"

exit 0

