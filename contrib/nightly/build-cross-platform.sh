#!/bin/bash

# This script is used to compile and tar gzip the release binaries so that they
# can be uploaded to Github. It would typically only be used by Dgraph developers
# while doing a new release. If you are looking to build Dgraph, you should run a
# go build from inside $GOPATH/src/github.com/dgraph-io/dgraph/cmd/dgraph

# Exit script in case an error is encountered.
set -e

echo -e "\n\n Downloading xgo"
go get github.com/karalabe/xgo

platform=$1
asset_suffix=$2
cur_dir=$(pwd);
tmp_dir=/tmp/dgraph-build;
release_version=$(git describe --abbrev=0);
if [[ -n $asset_suffix ]]; then
  release_version="$release_version${asset_suffix}"
fi

# TODO - Add checksum file later when we support get.dgraph.io for Windows.

# If temporary directory already exists delete it.
if [ -d "$tmp_dir" ]; then
  rm -rf $tmp_dir
fi

mkdir $tmp_dir;

source $GOPATH/src/github.com/dgraph-io/dgraph/contrib/nightly/constants.sh

pushd $GOPATH/src/github.com/dgraph-io/dgraph/dgraph > /dev/null

if [[ $platform == "windows" ]]; then
  xgo_target="windows/amd64"
else
  xgo_target="darwin-10.9/amd64"
fi

echo -e "\n\n\033[1;33mBuilding binaries for $platform\033[0m"
xgo --go 1.8.3 --targets $xgo_target -ldflags \
  "-X $release=$release_version -X $branch=$gitBranch -X $commitSHA1=$lastCommitSHA1 -X '$commitTime=$lastCommitTime' -X $uiDir=$ui" .;

echo -e "\n\033[1;33mCopying binaries to tmp folder\033[0m"
if [[ $platform == "windows" ]]; then
  cp dgraph-windows-4.0-amd64.exe $tmp_dir/dgraph.exe
else
  cp dgraph-darwin-10.9-amd64 $tmp_dir/dgraph
fi

echo -e "\n\033[1;34mSize of files: $(du -sh $tmp_dir)\033[0m"

echo -e "\n\033[1;33mCreating tar file\033[0m"
tar_file=dgraph-"$platform"-amd64-$release_version.tar.gz

# Create a tar file with the contents of the dgraph folder (i.e the binaries)
pushd $tmp_dir > /dev/null
if [[ $platform == "windows" ]]; then
  tar -zcvf $tar_file dgraph.exe
else
  tar -zcvf $tar_file dgraph
fi

echo -e "\n\033[1;34mSize of tar file: $(du -sh $tar_file)\033[0m"

echo -e "\n\033[1;33mMoving tarfile to original directory\033[0m"
mv $tar_file $cur_dir
popd > /dev/null
rm -rf $tmp_dir

