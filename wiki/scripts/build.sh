#!/bin/bash
# This script runs in a loop, checks for updates to the Hugo docs theme or
# to the docs on certain branches and rebuilds the public folder for them.
# It has be made more generalized, so that we don't have to hardcode versions.

# Warning - Changes should not be made on the server on which this script is running
# becauses this script does git checkout and merge.

set -e

GREEN='\033[32;1m'
RESET='\033[0m'
HOST=https://docs.dgraph.io

# TODO - Maybe get list of released versions from Github API and filter
# those which have docs.

# Place the latest version at the beginning so that version selector can
# append '(latest)' to the version string, and build script can place the
# artifact in an appropriate location
VERSIONS_ARRAY=(
'v0.9.2'
'master'
'v0.9.1'
'v0.9.0'
'v0.8.3'
'v0.8.2'
'v0.8.1'
'v0.8.0'
'v0.7.7'
'v0.7.6'
'v0.7.5'
'v0.7.4'
)

joinVersions() {
	versions=$(printf ",%s" "${VERSIONS_ARRAY[@]}")
	echo ${versions:1}
}

rebuild() {
	echo -e "$(date) $GREEN Updating docs for branch: $1.$RESET"

	# The latest documentation is generated in the root of /public dir
	# Older documentations are generated in their respective `/public/vx.x.x` dirs
	dir=''
	if [[ $2 != "${VERSIONS_ARRAY[0]}" ]]; then
		dir=$2
	fi

	VERSION_STRING=$(joinVersions)
	# In Unix environments, env variables should also be exported to be seen by Hugo
	export CURRENT_BRANCH=${1}
	export CURRENT_VERSION=${2}
	export VERSIONS=${VERSION_STRING}

	HUGO_TITLE="Dgraph Doc ${2}"\
		VERSIONS=${VERSION_STRING}\
		CURRENT_BRANCH=${1}\
		CURRENT_VERSION=${2} hugo\
		--destination=public/"$dir"\
		--baseURL="$HOST"/"$dir" 1> /dev/null
}

branchUpdated()
{
	local branch="$1"
	git checkout -q "$1"
	UPSTREAM=$(git rev-parse "@{u}")
	LOCAL=$(git rev-parse "@")

	if [ "$LOCAL" != "$UPSTREAM" ] ; then
		git merge -q origin/"$branch"
		return 0
	else
		return 1
	fi
}

publicFolder()
{
	dir=''
	if [[ $1 == "${VERSIONS_ARRAY[0]}" ]]; then
		echo "public"
	else
		echo "public/$1"
	fi
}

checkAndUpdate()
{
	local version="$1"
	local branch=""

	if [[ $version == "master" ]]; then
		branch="master"
	else
		branch="release/$version"
	fi

	if branchUpdated "$branch" ; then
		git merge -q origin/"$branch"
		rebuild "$branch" "$version"
	fi

	folder=$(publicFolder $version)
	if [ "$themeUpdated" = 0 ] || [ ! -d $folder ] ; then
		rebuild "$branch" "$version"
	fi
}

while true; do
	# Lets move to the docs directory.
	pushd /home/ubuntu/dgraph/wiki > /dev/null

	currentBranch=$(git rev-parse --abbrev-ref HEAD)

	# Lets check if the theme was updated.
	pushd themes/hugo-docs > /dev/null
	git remote update > /dev/null
	themeUpdated=1
	if branchUpdated "master" ; then
		echo -e "$(date) $GREEN Theme has been updated. Now will update the docs.$RESET"
		themeUpdated=0
	fi
	popd > /dev/null

	# Now lets check the theme.
	echo -e "$(date)  Starting to check branches."
	git remote update > /dev/null

	for version in "${VERSIONS_ARRAY[@]}"
	do
		checkAndUpdate "$version"
	done

	echo -e "$(date)  Done checking branches.\n"

	git checkout -q "$currentBranch"
	popd > /dev/null

	sleep 60
done
