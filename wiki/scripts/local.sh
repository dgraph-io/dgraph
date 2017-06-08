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

run() {
  export CURRENT_BRANCH=${CURRENT_BRANCH}
  export VERSIONS=${VERSION_STRING}

  HUGO_TITLE="Dgraph Doc - local" \
  VERSIONS=${VERSION_STRING} \
  CURRENT_BRANCH=${CURRENT_BRANCH} hugo server -w
}

run
