#!/bin/bash

set -e


lastCommitSHA1=$(git rev-parse --short HEAD);
gitBranch=$(git rev-parse --abbrev-ref HEAD)
lastCommitTime=$(git log -1 --format=%ci)
dgraph_cmd=$GOPATH/src/github.com/dgraph-io/dgraph/dgraph;
ui="/usr/local/share/dgraph/assets"

release="github.com/dgraph-io/dgraph/x.dgraphVersion"
branch="github.com/dgraph-io/dgraph/x.gitBranch"
commitSHA1="github.com/dgraph-io/dgraph/x.lastCommitSHA"
commitTime="github.com/dgraph-io/dgraph/x.lastCommitTime"
uiDir="main.uiDir"
