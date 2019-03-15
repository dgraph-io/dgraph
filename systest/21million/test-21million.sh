#!/bin/bash -e

readonly ME=${0##*/}
readonly SRCDIR=$(dirname $0)

QUERY_DIR=$SRCDIR/queries
BENCHMARKS_REPO="https://github.com/dgraph-io/benchmarks"
SCHEMA_URL="$BENCHMARKS_REPO/blob/master/data/21million.schema?raw=true"
DATA_URL="$BENCHMARKS_REPO/blob/master/data/21million.rdf.gz?raw=true"

function Info {
    echo -e "INFO: $*"
}

function DockerCompose {
    docker-compose -p dgraph "$@"
}

HELP= LOADER=bulk CLEANUP= FILE=

ARGS=$(getopt -n$ME -o"h" -l"help,loader:,cleanup:,file:" -- "$@") || exit 1
eval set -- "$ARGS"
while true; do
    case "$1" in
        -h|--help)  HELP=yes;              ;;
        --loader)   LOADER=${2,,}; shift   ;;
        --cleanup)  CLEANUP=${2,,}; shift  ;;
        --file)     FILE=$2; shift         ;;
        --)         shift; break           ;;
    esac
    shift
done

if [[ $HELP ]]; then
    cat <<EOF
usage: $ME [-h|--help] [--loader=<bulk|live|none>] [--cleanup=<all|none|servers>] [--file=<path>]

options:

    --loader        bulk=use dgraph bulk (default)
                    live=use dgraph live
                    none=use data volume from previous run
    --cleanup       all=take down containers and data volume when done (default)
                    servers=take down dgraph zero and alpha but leave data volume up
                    none=leave up containers and data volume when done
    --file          load a local data file instead of over the network
EOF
    exit 0
fi

if [[ $LOADER != bulk && $LOADER != live && $LOADER != none ]]; then
    echo >&2 "$ME: loader must be 'bulk' or 'live' or 'none' -- $LOADER"
    exit 1
fi

# default to leaving the data around for another run
# if already re-using it from a previous run
if [[ $LOADER == none && -z $CLEANUP ]]; then
    CLEANUP=servers
fi

# default to cleaning up both services and volume
if [[ -z $CLEANUP  ]]; then
    CLEANUP=all
elif [[ $CLEANUP != all && $CLEANUP != servers && $CLEANUP != none ]]; then
    echo >&2 "$ME: cleanup must be 'all' or 'servers' or 'none' -- $LOADER"
    exit 1
fi

# load a local data file instead of the one from the remote repo
unset LOCAL_VOL
if [[ $FILE ]]; then
    Info "loading local file $FILE"
    LOCAL_VOL="--volume $(readlink -f $FILE):/datafile"
    DATA_URL="file:///datafile"
fi

Info "entering directory $SRCDIR"
cd $SRCDIR

if [[ $LOADER != none ]]; then
    Info "removing old data (if any)"
    DockerCompose down --remove-orphans --volumes
else
    Info "using previously loaded data"
fi

Info "bringing up zero container"
DockerCompose up --remove-orphans --force-recreate --detach zero1

Info "waiting for zero to become leader"
DockerCompose logs -f zero1 | grep -q -m1 "I've become the leader"

if [[ $LOADER == bulk ]]; then
    Info "bulk loading data set"
    DockerCompose run --name bulk_load --rm $LOCAL_VOL dg1 \
        bash -s <<EOF
            /gobin/dgraph bulk --schema=<(curl -LSs $SCHEMA_URL) --files=<(curl -LSs $DATA_URL) \
                               --format=rdf --zero=zero1:5080 --out=/data/dg1/bulk
            mv /data/dg1/bulk/0/p /data/dg1
EOF
fi

Info "bringing up alpha container"
DockerCompose up --force-recreate --detach dg1

Info "waiting for alpha to be ready"
DockerCompose logs -f dg1 | grep -q -m1 "Server is ready"

if [[ $LOADER == live ]]; then
    Info "live loading data set"
    dgraph live --schema=<(curl -LSs $SCHEMA_URL) --files=<(curl -LSs $DATA_URL) \
                --format=rdf --zero=:5080 --dgraph=:9180 --logtostderr
fi

Info "running benchmarks/regression queries"
go test -v -tags standalone || FOUND_DIFFS=1

if [[ $CLEANUP == all ]]; then
    Info "bringing down zero and alpha and data volumes"
    DockerCompose down -v
elif [[ $CLEANUP != none ]]; then
    Info "bringing down zero and alpha only"
    DockerCompose down
fi

if [[ $FOUND_DIFFS -eq 0 ]]; then
    Info "no diffs found in query results"
else
    Info "found some diffs in query results"
fi

exit $FOUND_DIFFS
