#!/bin/bash
if [[ $TRAVIS_TAG == "nightly" ]]; then
  # We create nightly tag using the script so we don't want to run this script
  # when the tagged build is triggered.
  exit 0
fi

# We run a cron job daily on Travis which will update the nightly binaries.
if [[ $TRAVIS_EVENT_TYPE != "cron" ]]; then
   exit 0
fi

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
TAR_FILE="dgraph-${OS}-amd64-${DGRAPH_VERSION}-dev.tar.gz"
NIGHTLY_FILE="${GOPATH}/src/github.com/dgraph-io/dgraph/${TAR_FILE}"
SHA_FILE_NAME="dgraph-checksum-${OS}-amd64-${DGRAPH_VERSION}-dev.sha256"
SHA_FILE="${GOPATH}/src/github.com/dgraph-io/dgraph/${SHA_FILE_NAME}"
ASSETS_FILE="${GOPATH}/src/github.com/dgraph-io/dgraph/assets.tar.gz"

update_or_create_asset() {
  local release_id=$1
  local asset=$2
  local asset_file=$3
  local asset_id=$(send_gh_api_request repos/${DGRAPH_REPO}/releases/${release_id}/assets \
    | jq -r -c ".[] | select(.name == \"${asset}\").id")

  if [[ -n "${asset_id}" ]]; then
    echo "Found asset file for ${asset}. Deleting"
    send_gh_api_request repos/${DGRAPH_REPO}/releases/assets/${asset_id} \
    DELETE
  fi
  echo "Uplading asset ${asset}, loc: ${asset_file}"
  upload_release_asset ${asset_file} "$asset" \
  ${DGRAPH_REPO} ${release_id} \
  > /dev/null
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
  Dgraph development (pre-release) build which is updated every night.
  You can automatically install the nightly binaries along with the assets by running
  `curl https://nightly.dgraph.io -sSf | bash`.
  '
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
  fi

  # We upload the tar binary.
  local name="dgraph-${OS}-amd64-${DGRAPH_VERSION}-dev.tar.gz"
  update_or_create_asset $release_id $name ${NIGHTLY_FILE}

  local sha_name="dgraph-checksum-${OS}-amd64-${DGRAPH_VERSION}-dev.sha256"
  update_or_create_asset $release_id $sha_name ${SHA_FILE}


  if [[ $TRAVIS_OS_NAME == "linux" ]]; then
    # As asset would be the same on both platforms, we only upload it from linux.
    update_or_create_asset $release_id "assets.tar.gz" ${ASSETS_FILE}

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

# Dont check because Mac and Linux run in two separate scripts.
# nightly_sha=""
# read nightly_sha < <( \
#   send_gh_api_request repos/${DGRAPH_REPO}/git/refs/tags/${NIGHTLY_TAG} \
#   | jq -r '.object.sha') || true
#
# if [[ $nightly_sha == $DGRAPH_COMMIT ]]; then
#   echo "nightly $nightly_sha, dgraph commit $DGRAPH_COMMIT"
#   echo "Latest commit on master hasn't changed. Exiting"
#   exit 0
# fi
go get -u golang.org/x/net/context golang.org/x/text/unicode/norm google.golang.org/grpc

echo "Building embedded binaries"
contrib/releases/build.sh dev
upload_nightly

upload_docker_image
