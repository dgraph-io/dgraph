#!/bin/bash

set -e

GREEN='\033[32;1m'
RESET='\033[0m'

VERSIONS_ARRAY=(
  'preview'
)

joinVersions() {
  versions=$(printf ",%s" "${VERSIONS_ARRAY[@]}")
  echo "${versions:1}"
}

VERSION_STRING=$(joinVersions)

run() {
  export CURRENT_BRANCH="master"
  export CURRENT_VERSION=${VERSIONS_ARRAY[0]}
  export VERSIONS=${VERSION_STRING}
  export DGRAPH_ENDPOINT=${DGRAPH_ENDPOINT:-"https://play.dgraph.io/query?latency=true"}


  HUGO_TITLE="Dgraph Doc - Preview" \
  VERSIONS=${VERSION_STRING} \
  CURRENT_BRANCH="master" \

  pushd "$(dirname "$0")/.." > /dev/null
  pushd themes > /dev/null

  if [ ! -d "hugo-docs" ]; then
    echo -e "$(date) $GREEN  Cloning hugo-docs repository.$RESET"
    git submodule add https://github.com/dgraph-io/hugo-docs.git hugo-docs
  else
    echo -e "$(date) $GREEN  hugo-docs repository found. Pulling latest version.$RESET"
    pushd hugo-docs > /dev/null
    git pull
    popd > /dev/null
  fi
  popd > /dev/null


  CURRENT_VERSION=${CURRENT_VERSION} hugo \
    --destination=public 1> /dev/null
  popd > /dev/null
  echo -e "$(date) $GREEN  Done with creating the local build in public folder.$RESET"
}

run
