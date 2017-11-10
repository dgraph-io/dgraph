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
  "-X $release=$release_version -X $branch=$gitBranch -X $commitSHA1=$lastCommitSHA1 -X '$commitTime=$lastCommitTime' -X $uiDir=$ui" .;

echo -e "\n\033[1;34mSize of files before strip: $(du -sh dgraph)\033[0m"

strip -x dgraph
echo -e "\n\033[1;34mSize of files after  strip: $(du -sh dgraph)\033[0m"

digest_cmd=""
if hash shasum 2>/dev/null; then
  digest_cmd="shasum -a 256"
else
  echo -e "\033[0;31mYou don't have shasum command line tool available. Install it and try again.\033[0m"
  exit 1
fi

# Create the checksum file for dgraph binary.
checksum_file=$cur_dir/"dgraph-checksum-$platform-amd64-$release_version".sha256
if [ -f "$checksum_file" ]; then
	rm $checksum_file
fi

checksum=$($digest_cmd dgraph | awk '{print $1}')
echo "$checksum /usr/local/bin/dgraph" >> $checksum_file

# Move dgraph to tmp directory.
mv dgraph $tmp_dir

echo -e "\n\033[1;33mCreating tar file\033[0m"
tar_file=dgraph-"$platform"-amd64-$release_version
#popd &> /dev/null
popd

pushd $tmp_dir
# Create a tar file with the contents of the dgraph folder (i.e the binaries)
GZIP=-n tar -zcf $tar_file.tar.gz dgraph;
echo -e "\n\033[1;34mSize of tar file: $(du -sh $tar_file.tar.gz)\033[0m"

echo -e "\n\033[1;33mMoving tarfile to original directory\033[0m"
mv $tar_file.tar.gz $cur_dir

echo -e "Calculating and storing checksum for tar gzipped assets."
popd
mv $tmp_dir ./
GZIP=-n tar -zcf assets.tar.gz -C $GOPATH/src/github.com/dgraph-io/dgraph/dashboard/build .
checksum=$($digest_cmd assets.tar.gz | awk '{print $1}')
echo "$checksum /usr/local/share/dgraph/assets.tar.gz" >> $checksum_file

docker build -t dgraph/dgraph:nightly $GOPATH/src/github.com/dgraph-io/dgraph/contrib/nightly/Dockerfile

rm -Rf dgraph-build
