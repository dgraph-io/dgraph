#!/bin/bash

set -e
readonly ME=${0##*/}
readonly SRCDIR=$(dirname $0)

SCHEMA_C_FILE="1predicate.schema"
SCHEMA_FILE="1predicate-c.schema"
DATA_FILE="1predicate.rdf.gz"

function Info {
    echo -e "INFO: $*"
}

function DockerCompose {
    docker-compose -p dgraph "$@"
}
function DgraphLive {
    dgraph live --ludicrous "$@"
}

HELP= CLEANUP= SAVEDIR= LOAD_ONLY= QUIET=

ARGS=$(/usr/bin/getopt -n$ME -o"h" -l"help,cleanup:,savedir:,load-only,quiet" -- "$@") || exit 1
eval set -- "$ARGS"
while true; do
    case "$1" in
        -h|--help)      HELP=yes;              ;;
        --cleanup)      CLEANUP=${2,,}; shift  ;;
        --savedir)      SAVEDIR=${2,,}; shift  ;;
        --load-only)    LOAD_ONLY=yes          ;;
        --quiet)        QUIET=yes              ;;
        --)             shift; break           ;;
    esac
    shift
done

if [[ $HELP ]]; then
    cat <<EOF
usage: $ME [-h|--help] [--cleanup=<all|none|servers>] [--savedir=path] [--mode=<normal|ludicrous|none>]

options:

    --cleanup       all = take down containers and data volume (default)
                    servers = take down dgraph zero and alpha but leave data volume up
                    none = leave up containers and data volume
    --savedir=path  specify a directory to save test failure json in
                    for easier post-test review
    --load-only     load data but do not run tests
    --quiet         just report which queries differ, without a diff
EOF
    exit 0
fi


# default to cleaning up both services and volume
if [[ -z $CLEANUP  ]]; then
    CLEANUP=all
elif [[ $CLEANUP != all && $CLEANUP != servers && $CLEANUP != none ]]; then
    echo >&2 "$ME: cleanup must be 'all' or 'servers' or 'none'"
    exit 1
fi

# default to quiet mode if diffs are being saved in a directory
if [[ -n $SAVEDIR ]]; then
    QUIET=yes
fi

Info "entering directory $SRCDIR"
cd $SRCDIR

Info "removing old data (if any)"
DockerCompose down -v --remove-orphans

Info "bringing up zero container"
DockerCompose up -d --remove-orphans --force-recreate zero1

Info "waiting for zero to become leader"
DockerCompose logs -f zero1 | grep -q -m1 "I've become the leader"

Info "bringing up alpha container"
DockerCompose up -d --remove-orphans --force-recreate alpha1 alpha2 alpha3

Info "waiting for alpha to be ready"
DockerCompose logs -f alpha1 | grep -q -m1 "Server is ready"
# after the server prints the log "Server is ready", it may be still loading data from badger
Info "sleeping for 10 seconds for the server to be ready"
sleep 10

for i in {1..10}; do sleep 1; curl 'http://localhost:8180/alter' --data-binary $'@1predicate-c.schema'; echo "schema-c"$i; done & 
for i in {1..10}; do sleep 1; curl 'http://localhost:8180/alter' --data-binary $'@1predicate.schema'; echo "schema"$i; done & 

Info "live loading data set"
DgraphLive --schema=$SCHEMA_FILE --files=$DATA_FILE --format=rdf --zero=:5180 --alpha=:9180 --logtostderr --batch=1 &

wait;

sleep 10


if [[ $LOAD_ONLY ]]; then
    Info "exiting after data load"
    exit 0
fi

# replace variables if set with the corresponding option
SAVEDIR=${SAVEDIR:+-savedir=$SAVEDIR}
QUIET=${QUIET:+-quiet}

Info "running benchmarks/regression queries"

if [[ ! -z "$TEAMCITY_VERSION" ]]; then
    # Make TeamCity aware of Go tests
    export GOFLAGS="-json"
fi
go test -v -tags standalone $SAVEDIR $QUIET || FOUND_DIFFS=1

if [[ $FOUND_DIFFS -eq 0 ]]; then
    Info "no diffs found in query results"
else
    Info "Cluster logs for alpha1"
    docker logs alpha1
    Info "Cluster logs for alpha2"
    docker logs alpha2
    Info "Cluster logs for alpha3"
    docker logs alpha3
    Info "Cluster logs for zero1"
    docker logs zero1
    Info "found some diffs in query results"
fi

if [[ $CLEANUP == all ]]; then
    Info "bringing down zero and alpha and data volumes"
    DockerCompose down -v --remove-orphans
elif [[ $CLEANUP == none ]]; then
    Info "leaving up zero and alpha"
else
    Info "bringing down zero and alpha only"
    DockerCompose down --remove-orphans
fi

exit $FOUND_DIFFS
