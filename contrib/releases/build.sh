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
ui="/usr/local/share/dgraph/assets"

release="github.com/dgraph-io/dgraph/x.dgraphVersion"
branch="github.com/dgraph-io/dgraph/x.gitBranch"
commitSHA1="github.com/dgraph-io/dgraph/x.lastCommitSHA"
commitTime="github.com/dgraph-io/dgraph/x.lastCommitTime"
uiDir="main.uiDir"

echo -e "\033[1;33mBuilding binaries\033[0m"
for d in $dgraph_cmd/*; do
  n=$(basename "${d}")
  echo $n
  cd $d
  if [ "$n" = "dgraph" ];then
    go build -ldflags \
      "-X $release=$release_version -X $branch=$gitBranch -X $commitSHA1=$lastCommitSHA1 -X '$commitTime=$lastCommitTime' -X $uiDir=$ui" .;
  else
    go build -ldflags \
    "-X $release=$release_version -X $branch=$gitBranch -X $commitSHA1=$lastCommitSHA1 -X '$commitTime=$lastCommitTime'" .;
  fi
done

echo -e "\n\033[1;33mCopying binaries to tmp folder\033[0m"
cd $tmp_dir;
mkdir dgraph && pushd &> /dev/null dgraph;
# Stripping the binaries.
for d in $dgraph_cmd/*; do
  n=$(basename "${d}")
  strip $d/$n
  cp $d/$n .
done

echo -e "\n\033[1;34mSize of files after strip: $(du -sh)\033[0m"

digest_cmd=""
if hash shasum 2>/dev/null; then
  digest_cmd="shasum -a 256"
else
  echo -e "\033[0;31mYou don't have shasum command line tool available. Install it and try again.\033[0m"
  exit 1
fi

for d in $dgraph_cmd/*; do
  n=$(basename "${d}")
  checksum=$($digest_cmd $n | awk '{print $1}')
  echo "$checksum /usr/local/bin/$n" >> $checksum_file
done

echo -e "\n\033[1;33mCreating tar file\033[0m"
tar_file=dgraph-"$platform"-amd64-$release_version
popd &> /dev/null
# Create a tar file with the contents of the dgraph folder (i.e the binaries)
GZIP=-n tar -zcf $tar_file.tar.gz dgraph;
echo -e "\n\033[1;34mSize of tar file: $(du -sh $tar_file.tar.gz)\033[0m"

echo -e "\n\033[1;33mMoving tarfile to original directory\033[0m"
mv $tar_file.tar.gz $cur_dir
rm -rf $tmp_dir

echo -e "Calculating and storing checksum for tar gzipped assets."
cd $cur_dir
GZIP=-n tar -zcf assets.tar.gz -C $GOPATH/src/github.com/dgraph-io/dgraph/dashboard/build .
checksum=$($digest_cmd assets.tar.gz | awk '{print $1}')
echo "$checksum /usr/local/share/dgraph/assets.tar.gz" >> $checksum_file
