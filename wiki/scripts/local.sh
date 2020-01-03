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


  HUGO_TITLE="Dgraph Doc" \
  VERSIONS=${VERSION_STRING} \
  CURRENT_BRANCH="master" \

  pushd "$(dirname "$0")/.." > /dev/null
  pushd themes > /dev/null

  if [ ! -d "hugo-docs" ]; then
    echo -e "$(date) $GREEN  Adding hugo-docs submodule. $RESET"
    git submodule add https://github.com/dgraph-io/hugo-docs.git hugo-docs
  else
    echo -e "$(date) $GREEN  hugo-docs submodule found. Pulling latest changes from master branch. $RESET"
    git submodule update --init
    pushd hugo-docs > /dev/null
    git checkout master
    git pull
    popd > /dev/null
  fi
  popd > /dev/null


  if [[ $1 == "-p" || $1 == "--preview" ]]; then
    CURRENT_VERSION=${CURRENT_VERSION} hugo \
      --destination=public \
      --baseURL="$DEPLOY_PRIME_URL" 1> /dev/null
    popd > /dev/null
    echo -e "$(date) $GREEN  Done with creating the local build in public folder. $RESET"
  else
    CURRENT_VERSION=${CURRENT_VERSION} hugo server -w
  fi
}

run $1
