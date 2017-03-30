#!/bin/bash
# This script runs in a loop, checks for updates to the Hugo docs theme or
# to the wiki docs on certain branches and rebuilds the public folder for them.
# It has be made more generalized, so that we don't have to hardcode versions.

# Warning - Changes should not be made on the server on which this script is running
# becauses this script does git checkout and merge.

set -e

GREEN='\033[32;1m'
RESET='\033[0m'
HOST=https://docs.dgraph.io

rebuild() {
	echo -e "$(date) $GREEN Updating docs for branch: $1.$RESET"
	# Generate new docs after merging.
	HUGO_TITLE="Dgraph Doc ${2}" hugo\
		--destination=public/"$2"\
		--baseURL="$HOST"/"$2" 1> /dev/null
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

checkAndUpdate()
{
	local branch="$1"
	local tag="$2"

	if branchUpdated "$branch" ; then
		git merge -q origin/"$branch"
		rebuild "$branch" "$tag"
	fi

	if [ "$themeUpdated" = 0 ] || [ ! -d "public/$tag" ] ; then
		rebuild "$branch" "$tag"
	fi
}

while true; do
	# Lets move to the Wiki directory.
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
	checkAndUpdate "release/v0.7.4" "v0.7.4"
	checkAndUpdate "master" "master"
	echo -e "$(date)  Done checking branches.\n"

	git checkout -q "$currentBranch"
	popd > /dev/null

	sleep 60
done
