#!/bin/bash

set -eo pipefail

repo="dgraph-io/dgraph"


[ -z "$RELEASE_BRANCHES" ] && echo "Please set RELEASE_BRANCHES" && exit 0
[ -z "$GIT_EMAIL" ] && echo "Please set GIT_EMAIL" && exit 0
[ -z "$GIT_NAME" ] && echo "Please set GIT_NAME" && exit 0
[ -z "$GH_USERNAME" ] && echo "Please set GH_USERNAME" && exit 0
[ -z "$GH_TOKEN" ] && echo "Please set GH_TOKEN" && exit 0

if [[ ${RELEASE_BRANCHES[@]} == *","* ]]; then
    echo "Release branches should not contain commas. Set it as RELEASE_BRANCHES=('x' 'y')"
fi


TMP="/tmp/badger-update"
rm -Rf $TMP
mkdir $TMP

cd $TMP
git clone https://github.com/$repo

cd dgraph

git config user.name "$GIT_NAME"
git config user.email "$GIT_EMAIL"

for base in "${RELEASE_BRANCHES[@]}"
do
	# Ensure directory is clean before updating badger
	if [[ $(git diff --stat) != '' ]]; then
		echo 'Working directory dirty. Following changes were found'
		git --no-pager diff
		echo 'Exiting'
		exit 0
	fi

	git fetch origin $base
	git --no-pager branch
	git checkout origin/$base

	echo "Preparing for base branch $base"
	branch="$GH_USERNAME/$base-update-$(date +'%m/%d/%Y')"

	echo "Creating new branch $branch"
	git checkout -b $branch

	echo "Updating badger to master branch version"
	go get -v github.com/dgraph-io/badger/v2@master

	go mod tidy

	if [[ $(git diff --stat) == '' ]]; then
		echo 'No changes found.'
		echo 'Exiting'
		exit 0
	fi

	echo "Ready to commit following changes"
	git --no-pager diff

	git add go.mod go.sum

	message="$base: Update badger $(date +'%m/%d/%Y')"
	git commit -m "$message"

	# Set authentication credentials to allow "git push"
	git remote set-url origin https://${GH_USERNAME}:${GH_TOKEN}@github.com/$repo.git
	git push origin $branch
	echo "Done"

	# Create PR
	apiURL="https://api.github.com/repos/$repo/pulls"

	echo "Creating PR"
	body="{
		\"title\": \"${message}\",
		\"head\": \"${branch}\",
		\"base\": \"${base}\"
	}"

	PR_id=$(curl --silent -X POST -H "Authorization: Bearer $GH_TOKEN" -d "${body}" "${apiURL}" \
		| sed -n 's/.*"number": \(.*\),/\1/p' )

	[[ -z $PR_id ]] && echo "Failed to create PR" && exit 0

	echo "Created PR https://github.com/$repo/pull/${PR_id}"
	echo "DONE"
done
