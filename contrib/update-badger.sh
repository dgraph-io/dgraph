#!/bin/bash

set -eo pipefail

[ -z "$GIT_EMAIL" ] && echo "Please set GIT_EMAIL" && exit 0
[ -z "$GIT_NAME" ] && echo "Please set GIT_NAME" && exit 0
[ -z "$GH_USERNAME" ] && echo "Please set GH_USERNAME" && exit 0
[ -z "$GH_TOKEN" ] && echo "Please set GH_TOKEN" && exit 0

TMP="/tmp/badger-update"
rm -Rf $TMP
mkdir $TMP

cd $TMP
git clone --depth 1 https://github.com/dgraph-io/dgraph

cd dgraph

branch="update-$(date +'%m/%d/%Y')"
echo "Creating new branch $branch"
git checkout -b $branch

git config user.name "$GIT_NAME"
git config user.email "$GIT_EMAIL"

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

message="Update badger $(date +'%m/%d/%Y')"
git commit -m "$message"

# Set authentication credentials to allow "git push"
git remote set-url origin https://${GH_USERNAME}:${GH_TOKEN}@github.com/dgraph-io/dgraph.git
git push origin $branch
echo "Done"

# Create PR
apiURL="https://api.github.com/repos/dgraph-io/dgraph/pulls"

echo "Creating PR"
body="{
	\"title\": \"${message}\",
	\"head\": \"${branch}\",
	\"base\": \"master\"
}"

PR_id=$(curl --silent -X POST -H "Authorization: Bearer $GH_TOKEN" -d "${body}" "${apiURL}" \
    | sed -n 's/.*"number": \(.*\),/\1/p' )

[[ -z $PR_id ]] && echo "Failed to create PR" && exit 0

echo "Received PR id: ${PR_id}"
echo "DONE"
