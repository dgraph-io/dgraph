#!/bin/bash

CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD)
VERSIONS=(
  'v0.7.7'
  'v0.7.6'
  'master'
  'v0.7.5'
  'v0.7.4'
)

joinVersions() {
	versions=$(printf ",%s" "${VERSIONS[@]}")
	echo ${versions:1}
}

VERSION_STRING=$(joinVersions)

# ensure_doc_branch exits the script if doc is run on a branch not supported
ensure_doc_branch() {
  if [ $CURRENT_BRANCH != "master" ] && ! [[ $CURRENT_BRANCH =~ ^release\/v[0-9]\.[0-9]\.[0-9]$ ]]; then
    echo "You can only run documentation on 'master' or 'release/vx.x.x' branches."
    echo "See: https://github.com/dgraph-io/dgraph/tree/doc/wiki#branch"
    exit 1
  fi
}

run() {
  export CURRENT_BRANCH=${CURRENT_BRANCH}
  export VERSIONS=${VERSION_STRING}

  HUGO_TITLE="Dgraph Doc - local" \
  VERSIONS=${VERSION_STRING} \
  CURRENT_BRANCH=${CURRENT_BRANCH} hugo server -w
}

ensure_doc_branch
run
