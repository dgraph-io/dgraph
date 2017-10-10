#!/bin/bash

# This script is used to compile and tar gzip the release binaries so that they
# can be uploaded to Github. It would typically only be used by Dgraph developers
# while doing a new release. If you are looking to build Dgraph, you should run a
# go build from inside $GOPATH/src/github.com/dgraph-io/dgraph/cmd/dgraph

# Exit script in case an error is encountered.
set -e

docker pull karalabe/xgo-latest
go get github.com/karalabe/xgo

asset_suffix=$1
cur_dir=$(pwd);
tmp_dir=/tmp/dgraph-build;
release_version=$(git describe --abbrev=0);
if [[ -n $asset_suffix ]]; then
  release_version="$release_version${asset_suffix}"
fi
platform="windows"

# TODO - Add checksum file later when we support get.dgraph.io for Windows.

# If temporary directory already exists delete it.
if [ -d "$tmp_dir" ]; then
  rm -rf $tmp_dir
fi

mkdir $tmp_dir;

source $GOPATH/src/github.com/dgraph-io/dgraph/contrib/releases/constants.sh

xgo_target="windows/amd64"

echo -e "\n\n\033[1;33mBuilding binaries for $platform\033[0m"
for d in $dgraph_cmd/*; do
  n=$(basename "${d}")
  echo -e "\033[1;34m$n\033[0m"
  cd $d
  if [ "$n" = "dgraph" ];then
    xgo --go 1.8.3 --targets $xgo_target -ldflags \
      "-X $release=$release_version -X $branch=$gitBranch -X $commitSHA1=$lastCommitSHA1 -X '$commitTime=$lastCommitTime' -X $uiDir=$ui" .;
  else
    xgo --go 1.8.3 --targets $xgo_target -ldflags \
    "-X $release=$release_version -X $branch=$gitBranch -X $commitSHA1=$lastCommitSHA1 -X '$commitTime=$lastCommitTime'" .;
  fi
done

echo -e "\n\033[1;33mCopying binaries to tmp folder\033[0m"
cd $tmp_dir;
mkdir dgraph && pushd &> /dev/null dgraph;
# Stripping the binaries.
for d in $dgraph_cmd/*; do
  n=$(basename "${d}")
  cp $d/"$n-windows-4.0-amd64".exe "$n".exe
done

echo -e "\n\033[1;34mSize of files: $(du -sh)\033[0m"

echo -e "\n\033[1;33mCreating tar file\033[0m"
tar_file=dgraph-"$platform"-amd64-$release_version
popd &> /dev/null
# Create a tar file with the contents of the dgraph folder (i.e the binaries)
GZIP=-n tar -zcf $tar_file.tar.gz dgraph;
echo -e "\n\033[1;34mSize of tar file: $(du -sh $tar_file.tar.gz)\033[0m"

echo -e "\n\033[1;33mMoving tarfile to original directory\033[0m"
mv $tar_file.tar.gz $cur_dir
rm -rf $tmp_dir
