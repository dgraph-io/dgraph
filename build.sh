#!/bin/bash

cur_dir=$(pwd);
tmp_dir=/tmp/dgraph-build;
release_version=0.4.2;

# If temporary directory already exists delete it.
if [ -d "$tmp_dir" ]; then
  rm -rf $tmp_dir
fi

mkdir $tmp_dir;

dgraph_cmd=$GOPATH/src/github.com/dgraph-io/dgraph/cmd;
build_flags='-tags=embed'

echo -e "\033[1;33mBuilding binaries\033[0m"
echo "dgraph"
cd $dgraph_cmd/dgraph && go build $build_flags .;
echo "dgraphassigner"
cd $dgraph_cmd/dgraphassigner && go build $build_flags .;
echo "dgraphloader"
cd $dgraph_cmd/dgraphloader && go build $build_flags .;
echo "dgraphlist"
cd $dgraph_cmd/dgraphlist && go build $build_flags .;
echo "dgraphmerge"
cd $dgraph_cmd/dgraphmerge && go build $build_flags .;

echo -e "\n\033[1;33mCopying binaries to tmp folder\033[0m"
cd $tmp_dir;
cp $dgraph_cmd/dgraph/dgraph $dgraph_cmd/dgraphassigner/dgraphassigner $dgraph_cmd/dgraphlist/dgraphlist $dgraph_cmd/dgraphmerge/dgraphmerge $dgraph_cmd/dgraphloader/dgraphloader .;
# Stripping the binaries if strip command is available.
if type strip > /dev/null; then
	strip dgraph dgraphassigner dgraphloader dgraphmerge dgraphlist
fi
echo -e "\n\033[1;34mSize of files after strip: $(du -sh)\033[0m"

echo -e "\n\033[1;33mCreating tar file\033[0m"
tar_file=dgraph-"$(uname | tr '[:upper:]' '[:lower:]')"-amd64-v$release_version
tar -zcf $tar_file.tar.gz dgraph dgraphassigner dgraphlist dgraphmerge dgraphloader;
echo -e "\n\033[1;34mSize of tar file: $(du -sh $tar_file.tar.gz)\033[0m"

echo -e "\n\033[1;33mMoving tarfile to original directory\033[0m"
mv $tar_file.tar.gz $cur_dir
rm -rf $tmp_dir
