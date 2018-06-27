#!/bin/bash

# This script builds the dgraph binary with the version information, packages it into a tarball,
# uploads it to https://transfer.sh and prints the URL of the uploaded file. This URL can be
# supplied to Jepsen tests to use the current HEAD for the tests.

$GOPATH/src/github.com/dgraph-io/dgraph/contrib/nightly/build.sh "-dev"

echo -e "\nUploading the tar file to https://transfer.sh\n\n"
curl --upload-file dgraph-linux-amd64.tar.gz https://transfer.sh/dgraph-linux-amd64.tar.gz
