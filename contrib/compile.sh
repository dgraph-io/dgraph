#!/bin/bash

# This script is used to compile dgraph and dgraphloader with build flags.
# The build flags are useful in finding information about the binary.

release_version="$(git describe --abbrev=0)-dev";
lastCommitSHA1=$(git rev-parse --short HEAD);
gitBranch=$(git rev-parse --abbrev-ref HEAD)
lastCommitTime=$(git log -1 --format=%ci)
dgraph_cmd=$GOPATH/src/github.com/dgraph-io/dgraph/cmd;

release="github.com/dgraph-io/dgraph/x.dgraphVersion"
branch="github.com/dgraph-io/dgraph/x.gitBranch"
commitSHA1="github.com/dgraph-io/dgraph/x.lastCommitSHA"
commitTime="github.com/dgraph-io/dgraph/x.lastCommitTime"

echo -e "\033[1;33mBuilding binaries\033[0m"
echo "dgraph"
cd $dgraph_cmd/dgraph && \
   go build -ldflags \
   "-X $release=$release_version -X $branch=$gitBranch -X $commitSHA1=$lastCommitSHA1 -X '$commitTime=$lastCommitTime'" ;

echo "dgraphloader"
cd $dgraph_cmd/dgraphloader && \
   go build -ldflags \
   "-X $release=$release_version -X $branch=$gitBranch -X $commitSHA1=$lastCommitSHA1 -X '$commitTime=$lastCommitTime'" .;

echo "bulkloader"
cd $dgraph_cmd/bulkloader && \
   go build -ldflags \
   "-X $release=$release_version -X $branch=$gitBranch -X $commitSHA1=$lastCommitSHA1 -X '$commitTime=$lastCommitTime'" .;


