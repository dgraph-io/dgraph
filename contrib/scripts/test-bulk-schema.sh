#!/bin/bash
# verify fix of https://github.com/dgraph-io/dgraph/issues/2616

readonly ME=${0##*/}
readonly SRCDIR=$(readlink -f ${BASH_SOURCE[0]%/*})

declare -ri PORT_OFFSET=$((RANDOM % 1000))
declare -ri ZERO_PORT=$((5080+PORT_OFFSET))
declare -ri ALPHA_PORT=$((7080+PORT_OFFSET)) HTTP_PORT=$((8080+PORT_OFFSET))

INFO() { echo "$ME: $@"; }
ERROR() { echo >&2 "$ME: $@"; }
FATAL() { ERROR "$@"; exit 1; }

set -e

INFO "running bulk load schema test"

WORKDIR=$(mktemp --tmpdir -d $ME.tmp-XXXXXX)
INFO "using workdir $WORKDIR"
cd $WORKDIR

function StartZero
{
  INFO "starting zero server on port $ZERO_PORT"
  dgraph zero -o $PORT_OFFSET --my=localhost:$ZERO_PORT \
    >zero.log 2>&1 </dev/null &
  ZERO_PID=$!
  sleep 1
  $SRCDIR/../wait-for-it.sh -q -t 30 localhost:$ZERO_PORT \
    || FATAL "failed to start zero"
}

function BulkLoadSampleData
{
  INFO "bulk loading sample data"
  cat >1million.schema <<EOF
director.film: uid @reverse .
genre: uid @reverse .
initial_release_date: dateTime @index(year) .
name: string @index(term) @lang .
EOF
  mkfifo 1million.rdf.gz
  curl -LsS 'https://github.com/dgraph-io/tutorial/blob/master/resources/1million.rdf.gz?raw=true' >> 1million.rdf.gz &
  dgraph bulk -z localhost:$ZERO_PORT -s 1million.schema -r 1million.rdf.gz \
     >bulk.log 2>&1 </dev/null
}

function StartAlpha
{
  INFO "starting alpha server on port $ALPHA_PORT"
  dgraph alpha -o $PORT_OFFSET --my=localhost:$ALPHA_PORT --zero=localhost:$ZERO_PORT --lru_mb=2048 \
      >alpha.log 2>&1 </dev/null &
  ALPHA_PID=$!
  sleep 1
  $SRCDIR/../wait-for-it.sh -q -t 30 localhost:$ALPHA_PORT \
    || FATAL "failed to start alpha"
}

function UpdateDatabase
{
  INFO "adding predicate with default type to schema"
  curl localhost:$HTTP_PORT/alter -X POST -d$'
predicate_with_no_uid_count:string  .
predicate_with_default_type:default  .
predicate_with_index_no_uid_count:string @index(exact) .
' &>/dev/null

  curl localhost:$HTTP_PORT/mutate -X POST -H 'X-Dgraph-CommitNow: true' -d $'
{
  set {
    _:company1 <predicate_with_default_type> "CompanyABC" .
  }
}
' &>/dev/null
}

function QuerySchema
{
  INFO "running schema query"
  local out_file=${1:?no out file}
  curl -sS localhost:$HTTP_PORT/query -XPOST -d'schema(pred:[genre,language,name,revenue]) {}' | python -c "import json,sys; d=json.load(sys.stdin); json.dump(d['data'],sys.stdout,sort_keys=True,indent=2,separators=(',',': '))" > $out_file
  echo >> $out_file
  #INFO "schema is: " && cat $out_file
}

function DoExport
{
  INFO "running export"
  curl localhost:$HTTP_PORT/admin/export &>/dev/null
  sleep 1
}

function BulkLoadExportedData
{
  INFO "bulk loading exported data"
  dgraph bulk -z localhost:$ZERO_PORT \
              -s ../dir1/export/*/g01.schema.gz \
              -r ../dir1/export/*/g01.rdf.gz \
     >bulk.log 2>&1 </dev/null
  mv out/0/p .
}

function BulkLoadFixtureData
{
  INFO "bulk loading fixture data"

  # schema test cases:
  #
  # 1. predicate with non-default type (name)
  # 2. predicate with default type (genre)
  # 3. predicate not used in rdf (language)
  cat >fixture.schema <<EOF
name:string @index(term) .
genre:default .
language:string .
EOF

  # rdf test cases:
  #
  # 4. predicate not in schema (revenue)
  cat >fixture.rdf <<EOF
_:et <name> "E.T. the Extra-Terrestrial" .
_:et <genre> "Science Fiction" .
_:et <revenue> "792.9" .
EOF

  dgraph bulk -z localhost:$ZERO_PORT -s fixture.schema -r fixture.rdf \
     >bulk.log 2>&1 </dev/null
  mv out/0/p .
}

function StopServers
{
  INFO "killing zero server at pid $ZERO_PID"
  INFO "killing alpha server at pid $ALPHA_PID"
  kill $ZERO_PID $ALPHA_PID
  sleep 1
}

function Cleanup
{
  INFO "removing $WORKDIR"
  rm -rf $WORKDIR
}

mkdir dir1
pushd dir1 >/dev/null

StartZero
BulkLoadSampleData
StartAlpha
UpdateDatabase
QuerySchema "schema.out"
DoExport
StopServers

popd >/dev/null
mkdir dir2
pushd dir2 >/dev/null

StartZero
BulkLoadExportedData
StartAlpha
QuerySchema "schema.out"
StopServers

popd >/dev/null

INFO "verifing schema is same before export and after bulk import"
diff dir1/schema.out dir2/schema.out || FATAL "schema incorrect"
INFO "schema is correct"

mkdir dir3
pushd dir3 >/dev/null

StartZero
BulkLoadFixtureData
StartAlpha
QuerySchema "schema.out"
StopServers

popd >/dev/null

# final schema should include *all* predicates regardless of whether they were
# introduced by the schema or rdf file, used or not used, or of default type
# or non-default type
INFO "verifying schema contains all predicates"
diff - dir3/schema.out <<EOF || FATAL "schema incorrect"
{
  "schema": [
    {
      "predicate": "genre",
      "type": "default"
    },
    {
      "predicate": "language",
      "type": "string"
    },
    {
      "index": true,
      "predicate": "name",
      "tokenizer": [
        "term"
      ],
      "type": "string"
    },
    {
      "predicate": "revenue",
      "type": "default"
    }
  ]
}
EOF

INFO "schema is correct"

Cleanup

exit 0

# eof
