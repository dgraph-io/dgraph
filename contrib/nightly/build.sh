#!/bin/bash
set -e

# This script is run when
# 1. A cronjob is run on master which happens everyday and updates the nightly tag.
# 2. A new tag is pushed i.e. when we make a new release.
# 3. A release is updated.
run_upload_script() {
	if [[ $TRAVIS_TAG == "nightly" ]]; then
		# We create nightly tag using the script so we don't want to run this script
		# when the nightly build is triggered on updating where the commit points too.
		echo "Nightly tag. Skipping script"
		return 1
	fi

	# We run a cron job daily on Travis which will update the nightly binaries.
	if [ $TRAVIS_EVENT_TYPE == "cron" ] && [ "$TRAVIS" = true ]; then
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

BUILD_TAG=$(get_tag)
ASSET_SUFFIX=""

if [[ $BUILD_TAG == "nightly" ]]; then
	ASSET_SUFFIX="-dev"
fi

get_version() {
	# For nightly release, we find latest git tag.
	if [[ $BUILD_TAG == "nightly" ]]; then
		echo $(git describe --abbrev=0)
		return
	else
		# For an actual release, we get the version from the tag or the release branch.
		echo $BUILD_TAG
	fi
}

TRAVIS_EVENT_TYPE=${TRAVIS_EVENT_TYPE:-cron}
if ! run_upload_script; then
	echo "Skipping running the nightly script"
	exit 0
else
	echo "Running nightly script"
fi

if [[ $TRAVIS_OS_NAME == "osx" ]]; then
	OS="darwin"
else
	OS="linux"
fi

DGRAPH=$GOPATH/src/github.com/dgraph-io/dgraph
BUILD_DIR=$DGRAPH/contrib
source ${BUILD_DIR}/nightly/github.sh

DGRAPH_REPO="dgraph-io/dgraph"
DGRAPH_VERSION=$(get_version)
LATEST_TAG=$(git describe --abbrev=0)
DGRAPH_COMMIT=$(git rev-parse HEAD)
TAR_FILE="dgraph-${OS}-amd64-${DGRAPH_VERSION}${ASSET_SUFFIX}.tar.gz"
NIGHTLY_FILE="${GOPATH}/src/github.com/dgraph-io/dgraph/${TAR_FILE}"
SHA_FILE_NAME="dgraph-checksum-${OS}-amd64-${DGRAPH_VERSION}${ASSET_SUFFIX}.sha256"
SHA_FILE="${GOPATH}/src/github.com/dgraph-io/dgraph/${SHA_FILE_NAME}"
ASSETS_FILE="${GOPATH}/src/github.com/dgraph-io/dgraph/assets.tar.gz"
CURRENT_BRANCH=$TRAVIS_BRANCH
CURRENT_DIR=$(pwd)

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
	local name="dgraph-${OS}-amd64-${DGRAPH_VERSION}${ASSET_SUFFIX}.tar.gz"
	update_or_create_asset $release_id $name ${NIGHTLY_FILE}

	local sha_name="dgraph-checksum-${OS}-amd64-${DGRAPH_VERSION}${ASSET_SUFFIX}.sha256"
	update_or_create_asset $release_id $sha_name ${SHA_FILE}


	if [[ $TRAVIS_OS_NAME == "linux" ]]; then
		# As asset would be the same on both platforms, we only upload it from linux.
		update_or_create_asset $release_id "assets.tar.gz" ${ASSETS_FILE}

		# We dont want to update description programatically for version releases and commit apart from
		# nightly.
		if [[ $BUILD_TAG == "nightly" ]]; then
			echo 'Updating release description.'
			send_gh_api_data_request repos/${DGRAPH_REPO}/releases/${release_id} PATCH \
				"{ \"body\": $(get_nightly_release_body) | jq -s -c -R '.') }" \
				> /dev/null

			echo "Updating ${BUILD_TAG} tag to point to ${DGRAPH_COMMIT}."
			send_gh_api_data_request repos/${DGRAPH_REPO}/git/refs/tags/${BUILD_TAG} PATCH \
				"{ \"force\": true, \"sha\": \"${DGRAPH_COMMIT}\" }" \
				> /dev/null
		fi
	fi
}

DOCKER_TAG=""
docker_tag() {
	if [[ $BUILD_TAG == "nightly" ]]; then
		DOCKER_TAG=$CURRENT_BRANCH
	else
		DOCKER_TAG=$DGRAPH_VERSION
	fi
}

docker_tag

build_docker_image() {
	if [[ $TRAVIS_OS_NAME == "osx" ]]; then
		return 0
	fi

	pushd ${GOPATH}/src/github.com/dgraph-io/dgraph/contrib/nightly > /dev/null
	# Extract dgraph folder from the tar as its required by the Dockerfile.
	tar -xzf ${NIGHTLY_FILE}
	cp ${ASSETS_FILE} .
	echo -e "Building the docker image with tag: $DOCKER_TAG."
	docker build -t dgraph/dgraph:$DOCKER_TAG .
	if [[ $DOCKER_TAG == $LATEST_TAG ]]; then
		echo "Tagging docker image with latest tag"
		docker tag dgraph/dgraph:$DOCKER_TAG dgraph/dgraph:latest
	fi
	# Lets remove the dgraph folder now.
	rm -rf dgraph
	rm assets.tar.gz
}

upload_docker_image() {
	if [[ $TRAVIS_OS_NAME == "osx" ]]; then
		return 0
	fi

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
echo "Building embedded binaries"
contrib/releases/build.sh $ASSET_SUFFIX
build_docker_image

if [ "$TRAVIS" = true ]; then
	upload_assets
	upload_docker_image
fi

if [ "$DGRAPH" != "$CURRENT_DIR" ]; then
	mv $ASSETS_FILE $NIGHTLY_FILE $SHA_FILE $CURRENT_DIR
fi

popd > /dev/null
