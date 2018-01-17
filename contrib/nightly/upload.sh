#!/bin/bash
#
# This would build the binaries and docker image, and upload them to Github nightly build and Docker
# nightly tag.
# This also does the release, if run from the release branch on Travis.

set -e

# TODO (pawan) - This file declares a lot of redundant variables. Simplify it.
# This script is run when
# 1. A cronjob is run on master which happens everyday and updates the nightly tag.
# 2. A new tag is pushed i.e. when we make a new release.
# 3. A release is updated.

# TODO - Remove logic for step which updates the binaries for a release.
run_upload_script() {
  # So that script can run locally too.
  if [[ "$TRAVIS" != true ]]; then
    TRAVIS_BRANCH="master"
    return 0
  fi

  if [[ $TRAVIS_TAG == "nightly" ]]; then
		# We create nightly tag using the script so we don't want to run this script
		# when the nightly build is triggered on updating where the commit points too.
		echo "Nightly tag. Skipping script"
		return 1
	fi

	# We run a cron job daily on Travis which will update the nightly binaries.
	if [ $TRAVIS_EVENT_TYPE == "cron" ]; then
		if [ "$TRAVIS_BRANCH"  != "master" ];then
			echo "Cron job can only be run on master branch"
			return 1
		fi
		echo "Running nightly script for cron job."
		return 0
	fi

	branch=$TRAVIS_BRANCH
	release_reg="^release/(v.+)"
	if [[ $branch =~ $release_reg ]]; then
		tag=${BASH_REMATCH[1]}
		# If its the release branch and the tag already exists, then we want to update
		# the assets.
		echo $tag
		if git rev-parse $tag >/dev/null 2>&1
		then
			return 0
		else
			echo "On release branch, but tag doesn't exist. Skipping"
			return 1
		fi
	fi

	if [[ $TRAVIS_TAG =~ v.+ ]]; then
		echo "A new tag was pushed, running nightly script"
		return 0
	fi

	return 1
}

get_tag() {
	branch=$TRAVIS_BRANCH
	release_reg="^release/(v.+)"
	if [[ $branch =~ $release_reg ]]; then
		echo ${BASH_REMATCH[1]}
		return
	fi

	version="^v.+"
	if [[ $TRAVIS_TAG =~ $version ]]; then
		echo $TRAVIS_TAG
		return
	fi

	echo "nightly"
}

# Can either be of the type v0.x.y or be nightly.
BUILD_TAG=$(get_tag)
ASSET_SUFFIX=""

if [[ $BUILD_TAG == "nightly" ]]; then
	ASSET_SUFFIX="-dev"
fi

TRAVIS_EVENT_TYPE=${TRAVIS_EVENT_TYPE:-cron}
if ! run_upload_script; then
	echo "Skipping running the nightly script"
	exit 0
else
 declare -r SSH_FILE="$(mktemp -u $HOME/.ssh/XXXXX)"


  if [[ "$TRAVIS" == true ]]; then
    openssl aes-256-cbc \
      -K $encrypted_b471ec07d33f_key \
      -iv $encrypted_b471ec07d33f_iv \
      -in ".travis/ratel.enc" \
      -out "$SSH_FILE" -d

     chmod 600 "$SSH_FILE" \
      && printf "%s\n" \
        "Host github.com" \
        "  IdentityFile $SSH_FILE" \
        "  LogLevel ERROR" >> ~/.ssh/config
  fi

  go get -u github.com/jteeuwen/go-bindata/...
  pushd $GOPATH/src/github.com/dgraph-io
  if [[ ! -d ratel ]]; then
    git clone git@github.com:dgraph-io/ratel.git
  fi

  pushd ratel
  source ~/.nvm/nvm.sh
  nvm install --lts
  ./scripts/build.prod.sh
  popd
  popd

	echo "Running nightly script"
fi

OS="linux"

DGRAPH=$GOPATH/src/github.com/dgraph-io/dgraph
BUILD_DIR=$DGRAPH/contrib/nightly
source ${BUILD_DIR}/github.sh

DGRAPH_REPO="dgraph-io/dgraph"
DGRAPH_VERSION=$(git describe --abbrev=0)
LATEST_TAG=$(curl -s https://api.github.com/repos/dgraph-io/dgraph/releases/latest \
 | grep "tag_name" | awk '{print $2}' | tr -dc '[:alnum:]-.\n\r' | head -n1)
DGRAPH_COMMIT=$(git rev-parse HEAD)
TAR_FILE="dgraph-${OS}-amd64.tar.gz"
NIGHTLY_FILE="${GOPATH}/src/github.com/dgraph-io/dgraph/${TAR_FILE}"
OSX_NIGHTLY_FILE="${GOPATH}/src/github.com/dgraph-io/dgraph/dgraph-darwin-amd64.tar.gz"
SHA_FILE_NAME="dgraph-checksum-${OS}-amd64.sha256"
SHA_FILE="${GOPATH}/src/github.com/dgraph-io/dgraph/${SHA_FILE_NAME}"
OSX_SHA_FILE="${GOPATH}/src/github.com/dgraph-io/dgraph/dgraph-checksum-darwin-amd64.sha256"
CURRENT_BRANCH=$TRAVIS_BRANCH
CURRENT_DIR=$(pwd)

WINDOWS_TAR_NAME="dgraph-windows-amd64.tar.gz"
NIGHTLY_WINDOWS_FILE="${GOPATH}/src/github.com/dgraph-io/dgraph/$WINDOWS_TAR_NAME"

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

get_nightly_release_body() {
	echo '
	Dgraph development (pre-release) build which is updated every night.
	You can automatically install the nightly binaries along with the assets by running
	`curl https://nightly.dgraph.io -sSf | bash`.
	'
}

upload_assets() {
	local release_id
	# We check if a release with tag nightly already exists, else we create it.
	read release_id < <( \
		send_gh_api_request repos/${DGRAPH_REPO}/releases \
		| jq -r -c "(.[] | select(.tag_name == \"${BUILD_TAG}\").id), \"\"") \
		|| exit

	if [[ -z "${release_id}" ]]; then
		echo "Creating release for tag ${BUILD_TAG}."
		# For actual releases add draft true and for nightly release prerelease true.
		if [[ $BUILD_TAG == "nightly" ]]; then
			read release_id < <( \
				send_gh_api_data_request repos/${DGRAPH_REPO}/releases POST \
				"{ \"name\": \"Dgraph ${DGRAPH_VERSION}${ASSET_SUFFIX}\", \"tag_name\": \"${BUILD_TAG}\", \
				\"prerelease\": true }" \
				| jq -r -c '.id') \
				|| exit
		else
			read release_id < <( \
				send_gh_api_data_request repos/${DGRAPH_REPO}/releases POST \
				"{ \"name\": \"Dgraph ${DGRAPH_VERSION} Release\", \"tag_name\": \"${BUILD_TAG}\", \
				\"draft\": true }" \
				| jq -r -c '.id') \
				|| exit
		fi
	fi

	# We upload the tar binary.
	local name="dgraph-${OS}-amd64.tar.gz"
	update_or_create_asset $release_id $name ${NIGHTLY_FILE}

	local name="dgraph-darwin-amd64.tar.gz"
	update_or_create_asset $release_id $name ${OSX_NIGHTLY_FILE}

	local sha_name="dgraph-checksum-${OS}-amd64.sha256"
	update_or_create_asset $release_id $sha_name ${SHA_FILE}

	local sha_name="dgraph-checksum-darwin-amd64.sha256"
	update_or_create_asset $release_id $sha_name ${OSX_SHA_FILE}

	# As asset would be the same on both platforms, we only upload it from linux.
	update_or_create_asset $release_id $WINDOWS_TAR_NAME ${NIGHTLY_WINDOWS_FILE}

	# We dont want to update description programatically for version releases and commit apart from
	# nightly.
	if [[ $BUILD_TAG == "nightly" ]]; then
		echo 'Updating release description.'
		# TODO(pawan) - This fails, investigate and fix.
		# 			send_gh_api_data_request repos/${DGRAPH_REPO}/releases/${release_id} PATCH \
		# 				"{ \"force\": true, \"body\": $(get_nightly_release_body) | jq -s -c -R '.') }" \
		# 				> /dev/null
		#
		echo "Updating ${BUILD_TAG} tag to point to ${DGRAPH_COMMIT}."
		send_gh_api_data_request repos/${DGRAPH_REPO}/git/refs/tags/${BUILD_TAG} PATCH \
			"{ \"force\": true, \"sha\": \"${DGRAPH_COMMIT}\" }" \
			> /dev/null
	fi
}

DOCKER_TAG=""
docker_tag() {
	if [[ $BUILD_TAG == "nightly" ]]; then
		DOCKER_TAG="master"
	else
		DOCKER_TAG=$DGRAPH_VERSION
	fi
}

docker_tag

build_docker_image() {
	pushd $DGRAPH/contrib/nightly > /dev/null
	# Extract dgraph binary from the tar into dgraph-build folder.
	if [ ! -d dgraph-build ]; then
    mkdir dgraph-build
  fi
	tar -xzf ${NIGHTLY_FILE} -C dgraph-build
	echo -e "Building the docker image with tag: $DOCKER_TAG."
	docker build -t dgraph/dgraph:$DOCKER_TAG -f $DGRAPH/contrib/nightly/Dockerfile .
	if [[ $DOCKER_TAG == $LATEST_TAG ]]; then
		echo "Tagging docker image with latest tag"
		docker tag dgraph/dgraph:$DOCKER_TAG dgraph/dgraph:latest
	fi
  rm -rf dgraph
}

upload_docker_image() {
	echo "Logging into Docker."
	docker login -u="$DOCKER_USERNAME" -p="$DOCKER_PASSWORD"
	echo "Pushing the image"
	echo -e "Pushing image with tag $DOCKER_TAG"
	docker push dgraph/dgraph:$DOCKER_TAG
	if [[ $DOCKER_TAG == $LATEST_TAG ]]; then
		echo -e "Pushing latest image"
		docker push dgraph/dgraph:latest
	fi
	popd > /dev/null
}

pushd $DGRAPH > /dev/null

$BUILD_DIR/build-cross-platform.sh "windows" $ASSET_SUFFIX
$BUILD_DIR/build-cross-platform.sh "darwin" $ASSET_SUFFIX
$BUILD_DIR/build.sh $ASSET_SUFFIX

if [[ $DOCKER_TAG == "" ]]; then
  echo -e "No docker tag found. Exiting the script"
  exit 0
fi

build_docker_image

if [ "$TRAVIS" = true ]; then
	upload_assets
	upload_docker_image
fi

if [ "$DGRAPH" != "$CURRENT_DIR" ]; then
	mv $NIGHTLY_FILE $SHA_FILE $CURRENT_DIR
fi

# Lets rename the binaries before they are uploaded to S3.
mv $TRAVIS_BUILD_DIR/dgraph/dgraph $TRAVIS_BUILD_DIR/dgraph/dgraph-$TRAVIS_OS_NAME-${TRAVIS_COMMIT:0:7}

popd > /dev/null
