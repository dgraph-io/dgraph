#!/bin/bash

HOST=https://docs.dgraph.io

rebuild() {
	echo "$(date) Updating docs for branch: $1"
	# Generate new docs after merging.
	HUGO_TITLE="Dgraph Doc ${2}" hugo\
		--destination=public/$2\
		--baseURL=$HOST/$2 1> /dev/null
}

checkAndUpdate()
{
	local branch=$1
	git checkout -q $1
	local tag=$2

	UPSTREAM=$(git rev-parse "@{u}")
	LOCAL=$(git rev-parse "@")

	if [ $LOCAL != $UPSTREAM ] ; then
		git merge -q origin/$branch
		rebuild $branch $tag
	fi

	if [ ! -d "public/$tag" ] ; then
		rebuild $branch $tag
	fi
}

# Lets move to the Wiki directory.
cd /home/ubuntu/dgraph/wiki

while true; do
	echo -e "Starting to check branches\n"
	git remote update > /dev/null
	checkAndUpdate "release/v0.7.4" "v0.7.4"
	checkAndUpdate "master" "master"
	echo -e "Done checking branches\n"
	sleep 60
done
