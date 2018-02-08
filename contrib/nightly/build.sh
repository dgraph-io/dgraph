#!/bin/bash

# This script is used to compile and tar gzip the release binaries so that they
# can be uploaded to Github. It would typically only be used by Dgraph developers
# while doing a new release. If you are looking to build Dgraph, you should run a
# go build from inside $GOPATH/src/github.com/dgraph-io/dgraph/cmd/dgraph

# Exit script in case an error is encountered.
set -e

asset_suffix=$1
cur_dir=$(pwd);
tmp_dir=/tmp/dgraph-build;
release_version=$(git describe --abbrev=0);
if [[ -n $asset_suffix ]]; then
  release_version="$release_version${asset_suffix}"
fi
platform="$(uname | tr '[:upper:]' '[:lower:]')"
# If temporary directory already exists delete it.
if [ -d "$tmp_dir" ]; then
  rm -rf $tmp_dir
fi
mkdir $tmp_dir;

if ! type strip > /dev/null; then
  echo -e "\033[0;31mYou don't have strip command line tool available. Install it and try again.\033[0m"
  exit 1
fi

source $GOPATH/src/github.com/dgraph-io/dgraph/contrib/nightly/constants.sh
pushd $dgraph_cmd
echo -e "\033[1;33mBuilding dgraph binary for $platform\033[0m"
go build -ldflags \
  "-X $release=$release_version -X $branch=$gitBranch -X $commitSHA1=$lastCommitSHA1 -X '$commitTime=$lastCommitTime'" .;

strip -x dgraph

digest_cmd=""
if hash shasum 2>/dev/null; then
  digest_cmd="shasum -a 256"
else
  echo -e "\033[0;31mYou don't have shasum command line tool available. Install it and try again.\033[0m"
  exit 1
fi

# Create the checksum file for dgraph binary.
checksum_file=$cur_dir/"dgraph-checksum-$platform-amd64.sha256"
if [ -f "$checksum_file" ]; then
  rm $checksum_file
  rm -rf $cur_dir/"dgraph-checksum-darwin-amd64.sha256"
fi

checksum=$($digest_cmd dgraph | awk '{print $1}')
echo "$checksum /usr/local/bin/dgraph" >> $checksum_file

# Move dgraph to tmp directory.
cp dgraph $tmp_dir

popd

pushd $ratel
echo -e "\033[1;33mBuilding ratel binary for $platform\033[0m"
go build -ldflags \
  "-X $ratel_release=$release_version" -o dgraph-ratel
strip -x dgraph-ratel
checksum=$($digest_cmd dgraph-ratel | awk '{print $1}')
echo "$checksum /usr/local/bin/dgraph-ratel" >> $checksum_file
cp dgraph-ratel $tmp_dir

echo -e "\n\033[1;34mSize of files after  strip: $(du -sh $tmp_dir)\033[0m"

echo -e "\n\033[1;33mCreating tar file\033[0m"
tar_file=dgraph-"$platform"-amd64
#popd &> /dev/null
popd

# Create a tar file with the contents of the dgraph folder (i.e the binaries)
tar -zvcf $tar_file.tar.gz -C $tmp_dir .;
echo -e "\n\033[1;34mSize of tar file: $(du -sh $tar_file.tar.gz)\033[0m"

mv $tmp_dir ./

# Only run this locally.
if [[ $TRAVIS != true ]]; then
  docker build -t dgraph/dgraph:dev -f $GOPATH/src/github.com/dgraph-io/dgraph/contrib/nightly/Dockerfile .
fi

rm -Rf dgraph-build
