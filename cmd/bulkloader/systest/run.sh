#!/bin/bash

set -euo pipefail

script_dir=$(dirname $(readlink -f "$0"))

echo "Installing binaries..."
go install github.com/dgraph-io/dgraph/cmd/bulkloader
go install github.com/dgraph-io/dgraph/cmd/dgraph
go install github.com/dgraph-io/dgraph/cmd/dgraphzero
echo "Done."

fail=false
for suite in $script_dir/suite*; do
	echo Running test suite: $(basename $suite)

	rm -rf tmp
	mkdir tmp
	pushd tmp >/dev/null
	mkdir dg
	pushd dg >/dev/null
	$GOPATH/bin/bulkloader -r $suite/rdfs.rdf -s $suite/schema.txt >/dev/null 2>&1
	mv out/0 p
	popd >/dev/null

	mkdir dgz
	pushd dgz >/dev/null
	$GOPATH/bin/dgraphzero -id 1 >/dev/null 2>&1 &
	dgzPid=$!
	popd >/dev/null
	sleep 5

	pushd dg >/dev/null
	$GOPATH/bin/dgraph -peer localhost:8888 -memory_mb=1024 >/dev/null 2>&1 &
	dgPid=$!
	popd >/dev/null
	sleep 5

	popd >/dev/null # out of tmp
	result=$(curl --silent localhost:8080/query -XPOST -d @$suite/query.json)
	if ! $(jq --argfile a <(echo $result) --argfile b $suite/result.json -n 'def post_recurse(f): def r: (f | select(. != null) | r), .; r; def post_recurse: post_recurse(.[]?); ($a | (post_recurse | arrays) |= sort) as $a | ($b | (post_recurse | arrays) |= sort) as $b | $a == $b')
	then
		niceGot=$(echo $result | jq '.')
		niceWant=$(cat $suite/result.json | jq '.')
		echo "Actual result doesn't match expected result:"
		echo "Actual: $niceGot"
		echo "Expected: $niceWant"
		fail=true
	fi

	kill $dgPid
	kill $dgzPid
	sleep 2
done

if $fail; then
	exit 1
fi
