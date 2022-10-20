#!/bin/bash

# create checksum
create_checksum () {
  os=$1
  echo "Creating checksum for $os"
  if [[ "$os" != "windows" ]]; then
    pushd $TMP/$os/$GOARCH
      csum=$(shasum -a 256 dgraph | awk '{print $1}')
      echo $csum /usr/local/bin/dgraph >> ../dgraph-checksum-$os-$GOARCH.sha256
      csum=$(shasum -a 256 dgraph-ratel | awk '{print $1}')
      echo $csum /usr/local/bin/dgraph-ratel >> ../dgraph-checksum-$os-$GOARCH.sha256
    popd
  else
    pushd $TMP/$os/$GOARCH
      csum=$(shasum -a 256 dgraph.exe | awk '{print $1}')
      echo $csum dgraph.exe >> ../dgraph-checksum-$os-$GOARCH.sha256
      csum=$(shasum -a 256 dgraph-ratel.exe | awk '{print $1}')
      echo $csum dgraph-ratel.exe >> ../dgraph-checksum-$os-$GOARCH.sha256
    popd
  fi
}