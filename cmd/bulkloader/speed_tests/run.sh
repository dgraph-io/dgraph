#!/bin/bash

set -euo pipefail

scriptDir=$(dirname "$(readlink -f "$0")")

while [[ $# -gt 1 ]]; do
	key="$1"
	case $key in
		--tmp)
			tmp="$2"
			shift
			;;
		*)
			echo "unknown option $1"
			exit 1
			;;
	esac
	shift
done

tmp=${tmp:-tmp}

go install -race github.com/dgraph-io/dgraph/cmd/bulkloader

function run_test {
	[[ $# == 2 ]] || { echo "bad args"; exit 1; }
	schema=$1
	rdfs=$2

	rm -rf $tmp
	mkdir $tmp

	echo "$schema" > $tmp/sch.schema

	# Run bulk loader.
	$GOPATH/bin/bulkloader -map_shards=5 -reduce_shards=2 -shufflers=2 -mapoutput_mb=15 -tmp "$tmp/tmp" -out "$tmp/out" -l "$tmp/LEASE" -s "$tmp/sch.schema" -r "$rdfs"
}

echo "========================="
echo " 1 million data set      "
echo "========================="

run_test '
director.film:        uid @reverse @count .
genre:                uid @reverse .
initial_release_date: dateTime @index(year) .
name:                 string @index(term) .
starring:             uid @count .
' 1million.rdf.gz

echo "========================="
echo " 21 million data set     "
echo "========================="

run_test '
director.film        : uid @reverse @count .
actor.film           : uid @count .
genre                : uid @reverse @count .
initial_release_date : datetime @index(year) .
rating               : uid @reverse .
country              : uid @reverse .
loc                  : geo @index(geo) .
name                 : string @index(hash, fulltext, trigram) .
starring             : uid @count .
_share_hash_         : string @index(exact) .
' 21million.rdf.gz

echo "========================="
echo " Graph Overflow          "
echo "========================="

run_test '
AboutMe: string .
Author: uid @reverse .
Owner: uid @reverse .
DisplayName: string .
Location: string .
Reputation: int .
Score: int .
Text: string @index(fulltext) .
Tag.Text: string @index(hash) .
Type: string @index(exact) .
ViewCount: int @index(int) .
Vote: uid @reverse .
Title: uid @reverse .
Body: uid @reverse .
Post: uid @reverse .
PostCount: int @index(int) .
Tags: uid @reverse .
Timestamp: datetime .
GitHubID: string @index(hash) .
Has.Answer: uid @reverse @count .
Chosen.Answer: uid @count .
Comment: uid @reverse .
Upvote: uid @reverse .
Downvote: uid @reverse .
Tag: uid @reverse .
' comments.rdf.gz,posts.rdf.gz,tags.rdf.gz,users.rdf.gz,votes.rdf.gz
