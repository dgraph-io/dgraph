#!/bin/bash
if [[ $TRAVIS_TAG == "nightly" ]]; then
  # We create nightly tag using the script so we don't want to run this script
  # when the tagged build is triggered.
  exit 0
fi

# We run a cron job daily on Travis which will update the nightly binaries.
#if [[ $TRAVIS_EVENT_TYPE != "cron" ]]; then
#   exit 0
#fi

set -e
export PS4='+(${BASH_SOURCE}:${LINENO}): ${FUNCNAME[0]:+${FUNCNAME[0]}(): }'

if [[ $TRAVIS_OS_NAME == "osx" ]]; then
  OS="darwin"
else
  OS="linux"
fi

BUILD_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source ${BUILD_DIR}/nightly/github.sh

NIGHTLY_TAG="nightly"
DGRAPH_REPO="dgraph-io/dgraph"
DGRAPH_VERSION=$(git describe --abbrev=0)
DGRAPH_COMMIT=$(git rev-parse HEAD)
TAR_FILE="dgraph-${OS}-amd64-${DGRAPH_VERSION}.tar.gz"
NIGHTLY_FILE="${GOPATH}/src/github.com/dgraph-io/dgraph/${TAR_FILE}"
SHA_FILE_NAME="dgraph-checksum-${OS}-amd64-${DGRAPH_VERSION}.tar.gz"
SHA_FILE="${GOPATH}/src/github.com/dgraph-io/dgraph/${SHA_FILE_NAME}"
ASSETS_FILE="${GOPATH}/src/github.com/dgraph-io/dgraph/assets.tar.gz"

update_or_create_asset() {
  local release_id=$1
  local asset=$2
  local asset_id
    while read asset_id; do
      [[ -n "${asset_id}" ]] && echo "${asset_id}"
    done < <( \
      send_gh_api_request repos/${DGRAPH_REPO}/releases/${release_id}/assets \
      | jq -r -c '.[] | select(.name == "${asset}").id')

    if [[ -n "${asset_id}" ]]; then
	  echo "found asset"
    else
	    echo "not found, have to create"
    fi
}

delete_old_nightly() {
  local release_id
  read release_id < <( \
    send_gh_api_request repos/${DGRAPH_REPO}/releases \
    | jq -r -c "(.[] | select(.tag_name == \"${NIGHTLY_TAG}\").id), \"\"") \
    || exit

  if [[ ! -z "${release_id}" ]]; then
    echo "Deleting old nightly release"
    send_gh_api_request repos/${DGRAPH_REPO}/releases/${release_id} \
        DELETE \
        > /dev/null
  fi
}

get_release_body() {
  echo '
  Dgraph development (pre-release) build.

  You can run the following commands to run dgraph with the UI after downloading the assets.tar.gz and dgraph-linux64.tar.gz.
  ```
  mkdir -p ~/dgraph ~/dgraph/ui
  tar -C ~/dgraph -xzf dgraph-linux64.tar.gz --strip-components=1
  tar -C ~/dgraph/ui -xzf assets.tar.gz
  cd ~/dgraph
  dgraph --ui ui
  ```

  See **[Get Started](http://docs.dgraph.io/master/get-started/#step-2-run-dgraph)** for documentation.'
}

upload_nightly() {
  local release_id
  # We check if a release with tag nightly already exists, else we create it.
  read release_id < <( \
    send_gh_api_request repos/${DGRAPH_REPO}/releases \
    | jq -r -c "(.[] | select(.tag_name == \"${NIGHTLY_TAG}\").id), \"\"") \
    || exit

  if [[ -z "${release_id}" ]]; then
    echo "Creating release for tag ${NIGHTLY_TAG}."
    read release_id < <( \
      send_gh_api_data_request repos/${DGRAPH_REPO}/releases POST \
      "{ \"name\": \"Dgraph ${DGRAPH_VERSION}-dev\", \"tag_name\": \"${NIGHTLY_TAG}\", \
      \"prerelease\": true }" \
      | jq -r -c '.id') \
      || exit
  else
    # We upload the tar binary.
    echo "Uploading binaries. ${NIGHTLY_FILE} ${name}"
    local name="dgraph-${OS}-amd64-${DGRAPH_VERSION}-dev.tar.gz"
    upload_or_create_asset $release_id $name
    upload_release_asset ${NIGHTLY_FILE} "$name" \
      ${DGRAPH_REPO} ${release_id} \
      > /dev/null

    echo "Uploading shasum file. ${SHA_FILE} name ${sha_name}"
    local sha_name="dgraph-checksum-${OS}-amd64-${DGRAPH_VERSION}-dev.tar.gz"
    upload_or_create_asset $release_id $sha_name
    upload_release_asset ${SHA_FILE} "$sha_name" \
      ${DGRAPH_REPO} ${release_id} \
      > /dev/null


    if [[ $TRAVIS_OS_NAME == "linux" ]]; then
      echo 'Uploading assets file.'
      # As asset would be the same on both platforms, we only upload it from linux.
      upload_or_create_asset $release_id "assets.tar.gz"
      upload_release_asset ${ASSETS_FILE} "assets.tar.gz" \
        ${DGRAPH_REPO} ${release_id} \
        > /dev/null
    fi
  fi

  if [[ $TRAVIS_OS_NAME == "linux" ]]; then
    echo 'Updating release description.'
    send_gh_api_data_request repos/${DGRAPH_REPO}/releases/${release_id} PATCH \
      "{ \"body\": $(get_release_body | jq -s -c -R '.') }" \
      > /dev/null

    echo "Updating ${NIGHTLY_TAG} tag to point to ${DGRAPH_COMMIT}."
    send_gh_api_data_request repos/${DGRAPH_REPO}/git/refs/tags/${NIGHTLY_TAG} PATCH \
      "{ \"force\": true, \"sha\": \"${DGRAPH_COMMIT}\" }" \
      > /dev/null

  fi
}

upload_docker_image() {
  if [[ $TRAVIS_OS_NAME == "osx" ]]; then
    return 0
  fi

  pushd ${GOPATH}/src/github.com/dgraph-io/dgraph/contrib/nightly > /dev/null
  # Extract dgraph folder from the tar as its required by the Dockerfile.
  tar -xzf ${NIGHTLY_FILE}
  cp ${ASSETS_FILE} .
  echo "Building the dgraph master image."
  docker build -t dgraph/dgraph:master .
  echo "Logging into Docker."
  docker login -u="$DOCKER_USERNAME" -p="$DOCKER_PASSWORD"
  echo "Pushing the image"
  docker push dgraph/dgraph:master
  popd > /dev/null
}

nightly_sha=""
echo "tag ${NiGHTLY_TAG}"
read nightly_sha < <( \
  send_gh_api_request repos/${DGRAPH_REPO}/git/refs/tags/${NIGHTLY_TAG} \
  | jq -r '.object.sha') || true

echo "here"
if [[ $nightly_sha == $DGRAPH_COMMIT ]]; then
  echo "nightly $nightly_sha, dgraph commit $DGRAPH_COMMIT"
  echo "Latest commit on master hasn't changed. Exiting"
  exit 0
fi
go get -u golang.org/x/net/context golang.org/x/text/unicode/norm google.golang.org/grpc

echo "Building embedded binaries"
# contrib/releases/build.sh
# delete_old_nightly
upload_nightly

upload_docker_image
