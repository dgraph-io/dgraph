#!/bin/bash

# This script is used to compile and tar gzip the release binaries so that they
# can be uploaded to Github. It would typically only be used by Dgraph developers
# while doing a new release. If you are looking to build Dgraph, you should run a
# go build from inside $GOPATH/src/github.com/dgraph-io/dgraph/cmd/dgraph

# Exit script in case an error is encountered.
set -e

cur_dir=$(pwd);
tmp_dir=/tmp/dgraph-build;
release_version=$(git describe --abbrev=0);
platform="$(uname | tr '[:upper:]' '[:lower:]')"
checksum_file=$cur_dir/"dgraph-checksum-$platform-amd64-$release_version".sha256
if [ -f "$checksum_file" ]; then
	rm $checksum_file
fi

# If temporary directory already exists delete it.
if [ -d "$tmp_dir" ]; then
  rm -rf $tmp_dir
fi

if ! type strip > /dev/null; then
  echo -e "\033[0;31mYou don't have strip command line tool available. Install it and try again.\033[0m"
  exit 1
fi

mkdir $tmp_dir;

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
   go build -v -tags=embed -ldflags \
   "-X $release=$release_version -X $branch=$gitBranch -X $commitSHA1=$lastCommitSHA1 -X '$commitTime=$lastCommitTime'" .;
echo "dgraphloader"
echo $release
cd $dgraph_cmd/dgraphloader && \
   go build -v -tags=embed -ldflags \
   "-X $release=$release_version -X $branch=$gitBranch -X $commitSHA1=$lastCommitSHA1 -X '$commitTime=$lastCommitTime'" .;

echo -e "\n\033[1;33mCopying binaries to tmp folder\033[0m"
cd $tmp_dir;
mkdir dgraph && pushd &> /dev/null dgraph;
cp $dgraph_cmd/dgraph/dgraph $dgraph_cmd/dgraphloader/dgraphloader .;

# Stripping the binaries.
# Stripping binaries on Mac doesn't lead to much reduction in size and
# instead gives an error.
if [ "$platform" = "linux" ]; then
  strip dgraph dgraphloader
  echo -e "\n\033[1;34mSize of files after strip: $(du -sh)\033[0m"
fi

digest_cmd=""
if hash shasum 2>/dev/null; then
  digest_cmd="shasum -a 256"
else
  echo -e "\033[0;31mYou don't have shasum command line tool available. Install it and try again.\033[0m"
  exit 1
fi

checksum=$($digest_cmd dgraph | awk '{print $1}')
echo "$checksum /usr/local/bin/dgraph" >> $checksum_file
checksum=$($digest_cmd dgraphloader | awk '{print $1}')
echo "$checksum /usr/local/bin/dgraphloader" >> $checksum_file

echo -e "\n\033[1;33mCreating tar file\033[0m"
tar_file=dgraph-"$platform"-amd64-$release_version
popd &> /dev/null
# Create a tar file with the contents of the dgraph folder (i.e the binaries)
tar -zcf $tar_file.tar.gz dgraph;
echo -e "\n\033[1;34mSize of tar file: $(du -sh $tar_file.tar.gz)\033[0m"

echo -e "\n\033[1;33mMoving tarfile to original directory\033[0m"
mv $tar_file.tar.gz $cur_dir
rm -rf $tmp_dir

echo -e "Calculating and storing checksum for tar gzipped assets."
cd $cur_dir
tar -zcf assets.tar.gz -C $GOPATH/src/github.com/dgraph-io/dgraph/dashboard/build .
checksum=$($digest_cmd assets.tar.gz | awk '{print $1}')
echo "$checksum /usr/local/share/dgraph/assets.tar.gz" >> $checksum_file
