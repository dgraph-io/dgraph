#!/bin/bash

VERSIONS_ARRAY=(
  'v0.9.0'
  'master'
  'v0.8.3'
)

joinVersions() {
	versions=$(printf ",%s" "${VERSIONS_ARRAY[@]}")
	echo ${versions:1}
}

VERSION_STRING=$(joinVersions)

run() {
  export CURRENT_BRANCH="master"
  export CURRENT_VERSION=${VERSIONS_ARRAY[0]}
  export VERSIONS=${VERSION_STRING}


  HUGO_TITLE="Dgraph Doc - local" \
  VERSIONS=${VERSION_STRING} \
  CURRENT_BRANCH="master" \
  pushd $GOPATH/src/github.com/dgraph-io/dgraph/wiki > /dev/null

  pushd themes > /dev/null
  if [ ! -d "hugo-docs" ]; then
    git clone https://github.com/dgraph-io/hugo-docs
  else
    pushd hugo-docs > /dev/null
    git pull
    popd > /dev/null
  fi
  popd > /dev/null


  CURRENT_VERSION=${CURRENT_VERSION} hugo server -w
  popd > /dev/null
}

run
