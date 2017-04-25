#!/bin/bash
# This script runs as a cron job. It builds dgraph, dgraphloader and loads up the
# 21million.rdf.gz dataset when invoked. It would be used to display proper responses
# for docs on master branch.

set -e

GREEN='\033[32;1m'
RED='\033[91;1m'
RESET='\033[0m'

GOPATH=$HOME/go
PATH=$HOME/bin:/home/ubuntu/.local/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/local/go/bin:/home/ubuntu/go/bin

checkDir()
{
	if [ ! -d "$1" ]; then
		echo -e "$(date)$RED $1 not found. Exiting$RESET"
		exit 1
	fi
}

checkFile()
{
	if [ ! -f "$1" ]; then
		echo -e "$(date)$RED $1 not found. Exiting$RESET"
		exit 1
	fi
}

updateBranch()
{
	if [[ `git status --porcelain` ]]; then
		# This means there are some local changes, which shouldn't happen as code
		# shouldn't be modified on the server, lets exit.
		echo -e "$(date)$RED Found local changes in $(pwd), exiting.$RESET"
		exit 1
	fi

	# Pulling in the latest master changes.
	branch="master"
	git checkout -q $branch
	git merge -q origin/"$branch"
}

dgraphRepo=$GOPATH/src/github.com/dgraph-io/dgraph
benchmarksRepo=$GOPATH/src/github.com/dgraph-io/benchmarks
schema="data/goldendata.schema"
schemaPath="$benchmarksRepo/$schema"
data="data/goldendata.rdf.gz"
dataPath="$benchmarksRepo/$data"

checkDir $dgraphRepo
checkDir $benchmarksRepo
checkFile $schemaPath
checkFile $dataPath

pushd $benchmarksRepo > /dev/null
# Looking for changes to schema or data file.
updateBranch
popd > /dev/null

pushd $dgraphRepo > /dev/null
updateBranch
popd > /dev/null

echo "$(date) Building dgraph and dgraphloader"
cd $dgraphRepo/cmd/dgraph && go build .
cd $dgraphRepo/cmd/dgraphloader && go build .
echo -e "$(date)$GREEN dgraph and dgraphloader built successfully. $RESET"

latestCommit=$(git rev-parse --short HEAD)
unixTs=$(date +%s)
dgraphDir="$latestCommit-$unixTs"
mkdir -p ~/dgraph/$dgraphDir && cd ~/dgraph/$dgraphDir
# Starting Dgraph in background and outputting log to file.
$dgraphRepo/cmd/dgraph/dgraph --port 8082 --workerport 12346 > dgraph.log 2>&1 &
sleep 15
echo -e "$(date) Started Dgraph on port 8082. Now loading the dataset."
$dgraphRepo/cmd/dgraphloader/dgraphloader --d 127.0.0.1:8082 --s $schemaPath \
	--r $dataPath
echo -e "$(date)$GREEN Data loaded successfully. $RESET"

# Lets shutdown old and new Dgraph instance. Then restart new instance with nomutations
# flag.
curl -s localhost:80/admin/shutdown
curl -s localhost:8082/admin/shutdown

sleep 10
sudo $dgraphRepo/cmd/dgraph/dgraph --bindall=true --nomutations=true --port 80 \
	--workerport 12345 -ui $dgraphRepo/dashboard/build > dgraph.log 2>&1 &
echo -e "$(date)$GREEN Started Dgraph on port 80.$RESET"

cd ~/dgraph
# Delete all folders except the latest commit and its p and w directory.
ls | grep -v $dgraphDir | xargs sudo rm -rf