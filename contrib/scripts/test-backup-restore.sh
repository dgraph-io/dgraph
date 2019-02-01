#!/bin/bash

readonly ME=${0##*/}
readonly SRCDIR=$(readlink -f ${BASH_SOURCE[0]%/*})
# readonly SRCDIR=$(dirs -l)
readonly DATADIR=$GOPATH/src/github.com/dgraph-io/dgraph/systest/data

declare -ri PORT_OFFSET=$((RANDOM % 1000))
declare -ri ZERO_PORT=$((5080+PORT_OFFSET))
declare -ri ALPHA_PORT=$((7080+PORT_OFFSET)) HTTP_PORT=$((8080+PORT_OFFSET))

INFO() { echo "$ME: $@"; }
ERROR() { echo >&2 "$ME: $@"; }
FATAL() { ERROR "$@"; exit 1; }

set -e

INFO "running backup restore test"

WORKDIR=$(mktemp --tmpdir -d $ME.tmp-XXXXXX)
# WORKDIR=$(mktemp -d $TMPDIR/$ME.tmp-XXXXXX)
INFO "using workdir $WORKDIR"
cd $WORKDIR

function StartZero
{
  INFO "starting zero server on port $ZERO_PORT"
  dgraph zero -o $PORT_OFFSET --my=localhost:$ZERO_PORT --enterprise_features \
    >zero.log 2>&1 </dev/null &
  ZERO_PID=$!
  sleep 1
  $SRCDIR/../wait-for-it.sh -q -t 30 localhost:$ZERO_PORT \
    || FATAL "failed to start zero"
}

function BackupLoadSampleData
{
  INFO "backup loading sample data"
  dgraph bulk -z localhost:$ZERO_PORT \
    -s $DATADIR/goldendata.schema \
    -r $DATADIR/goldendata_first_200k.rdf.gz \
    >bulk.log 2>&1 </dev/null \
    || FATAL "failed to load sample data"
  sleep 1
  mv out/0/p .
}

function StartAlpha
{
  INFO "starting alpha server on port $ALPHA_PORT"
  dgraph alpha -o $PORT_OFFSET --my=localhost:$ALPHA_PORT --zero=localhost:$ZERO_PORT \
    --lru_mb=2048 \
    --enterprise_features \
    >alpha.log 2>&1 </dev/null &
  ALPHA_PID=$!
  sleep 1
  $SRCDIR/../wait-for-it.sh -q -t 30 localhost:$ALPHA_PORT \
    || FATAL "failed to start alpha"
}

function BackupRequest
{
  INFO "requesting backup"
  curl -XPOST localhost:$HTTP_PORT/admin/backup -F"destination=$WORKDIR/dir1" &>/dev/null
  sleep 10
}

function BackupRequestAt
{
  local dir=${1:?no dir given}
  local readts=${2:?no ts given}
  INFO "requesting incremental backup at $readts"
  curl -XPOST localhost:$HTTP_PORT/admin/backup -F"destination=$WORKDIR/$dir" -F"at=$readts" &>/dev/null
  sleep 10
}

function BackupRestore
{
  local dir1=${1:?no dir given}
  local dir2=${2:?no dir given}
  INFO "backup loading data"
  dgraph restore \
    -l $WORKDIR/$dir1/ \
    -p $WORKDIR/$dir2/ \
    >restore.log 2>&1 </dev/null \
    || FATAL "backup restore failed"
  sleep 1
}

function CheckPData
{
  local dir=${1:?no dir given}
  dgraph debug \
    -p $dir \
    >check.log 2>&1 </dev/null \
    || FATAL "backup failed p data check"
  sleep 1
  echo $(tail check.log | grep -o 'Found [0-9]\+ keys')
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

StartZero
BackupLoadSampleData
StartAlpha

mkdir dir1
pushd dir1 >/dev/null
BackupRequest
popd >/dev/null

# checked later
mkdir dir3
pushd dir3 >/dev/null
BackupRequestAt "dir3" "2"
popd >/dev/null

StopServers

mkdir dir2
pushd dir2 >/dev/null
BackupRestore "dir1" "dir2"
popd >/dev/null

p0=$(CheckPData "$WORKDIR/p/")
p1=$(CheckPData "$WORKDIR/dir2/p1/")

[ "$p0" != "$p1" ] && FATAL "Restore failed. Expected '$p0' but got '$p1'"

INFO "restore was successful"

INFO "running incremental backup test"

mkdir dir4
pushd dir4 >/dev/null
BackupRestore "dir3" "dir4"
popd >/dev/null

p2=$(CheckPData "$WORKDIR/dir4/p1/")

[ "$p0" != "$p2" ] && FATAL "Restore from incr failed. Expected '$p0' but got '$p2'"

INFO "incremental backup was successful"

Cleanup

exit 0
