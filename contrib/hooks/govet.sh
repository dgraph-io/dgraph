#!/bin/bash


#!/bin/sh
# Copyright 2012 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# git go vet pre-commit hook
#
# To use, store as .git/hooks/pre-commit inside your repository and make sure
# it has execute permissions.

if [ -z "$GOPATH" ]; then
	echo "ERROR: pre-commit hook for go vet: \$GOPATH is empty. Please run 'source dev.env' to set the correct \$GOPATH."
	exit 1
fi

# This script does not handle file names that contain spaces.
gofiles=$(git diff --cached --name-only --diff-filter=ACM | grep '\.go$' | grep -v '^vendor/')

# If any checks are found to be useless, they can be disabled here.
# See the output of "go tool vet" for a list of flags.
vetflags="-all=true"

errors=

# Run on one file at a time because a single invocation of "go tool vet"
# with multiple files requires the files to all be in one package.
for gofile in $gofiles
do
	if ! go tool vet $vetflags $gofile 2>&1; then
		errors=YES
	fi
done

[ -z  "$errors" ] && exit 0

echo
echo "Please fix the go vet warnings above. To disable certain checks, change vetflags in misc/git/hooks/govet."
exit 1

