#!/bin/bash -e

readonly ME=${0##*/}
readonly SRCDIR=$(dirname $0)

QUERY_DIR=$SRCDIR/queries
BENCHMARKS_REPO="https://github.com/dgraph-io/benchmarks"
SCHEMA_URL="$BENCHMARKS_REPO/blob/master/data/21million.schema?raw=true"
DATA_URL="$BENCHMARKS_REPO/blob/master/data/21million.rdf.gz?raw=true"

# this may be used to load a smaller data set when testing the test itself
#DATA_URL="$BENCHMARKS_REPO/blob/master/data/goldendata.rdf.gz?raw=true"

function Info {
    echo -e "INFO: $*"
}

function DockerCompose {
    docker-compose -p dgraph "$@"
}

HELP= LOADER=bulk CLEANUP= SAVEDIR= LOAD_ONLY= QUIET=

ARGS=$(/usr/bin/getopt -n$ME -o"h" -l"help,loader:,cleanup:,savedir:,load-only,quiet" -- "$@") || exit 1
eval set -- "$ARGS"
while true; do
    case "$1" in
        -h|--help)      HELP=yes;              ;;
        --loader)       LOADER=${2,,}; shift   ;;
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
usage: $ME [-h|--help] [--loader=<bulk|live|none>] [--cleanup=<all|none|servers>] [--savedir=path]

options:

    --loader        bulk = use dgraph bulk (default)
                    live = use dgraph live
                    none = use data loaded by previous run
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

# default to quiet mode if diffs are being saved in a directory
if [[ -n $SAVEDIR ]]; then
    QUIET=yes
fi

# check data version to help distinguish diffs due to a change in the data
# rather than a change in the code
if [[ $LOADER != none ]]; then
    Info "checking data set version"
    VERSION_FILE=$(mktemp --tmpdir)
    trap "rm -f $VERSION_FILE" EXIT
    curl -LSs --head $SCHEMA_URL | awk 'toupper($0)~/^ETAG:/ {print "Schema:"$2}' >> $VERSION_FILE
    curl -LSs --head $DATA_URL | awk 'toupper($0)~/^ETAG:/ {print "Data:"$2}' >> $VERSION_FILE
    diff -bi $VERSION_FILE queries/data-version || true
fi

Info "entering directory $SRCDIR"
cd $SRCDIR

if [[ $LOADER != none ]]; then
    Info "removing old data (if any)"
    DockerCompose down -v --remove-orphans
else
    Info "using previously loaded data"
fi

Info "bringing up zero container"
DockerCompose up -d --remove-orphans --force-recreate zero1

Info "waiting for zero to become leader"
DockerCompose logs -f zero1 | grep -q -m1 "I've become the leader"

if [[ $LOADER == bulk ]]; then
    Info "bulk loading data set"
    DockerCompose run --name bulk_load --rm alpha1 \
        bash -s <<EOF
            /gobin/dgraph bulk --schema=<(curl -LSs $SCHEMA_URL) --files=<(curl -LSs $DATA_URL) \
                               --format=rdf --zero=zero1:5080 --out=/data/alpha1/bulk
            mv /data/alpha1/bulk/0/p /data/alpha1
EOF
fi

Info "bringing up alpha container"
DockerCompose up -d --force-recreate alpha1

Info "waiting for alpha to be ready"
DockerCompose logs -f alpha1 | grep -q -m1 "Server is ready"

if [[ $LOADER == live ]]; then
    Info "live loading data set"
    dgraph live --schema=<(curl -LSs $SCHEMA_URL) --files=<(curl -LSs $DATA_URL) \
                --format=rdf --zero=:5080 --alpha=:9180 --logtostderr
fi

if [[ $LOAD_ONLY ]]; then
    Info "exiting after data load"
    exit 0
fi

# replace variables if set with the corresponding option
SAVEDIR=${SAVEDIR:+-savedir=$SAVEDIR}
QUIET=${QUIET:+-quiet}

Info "running benchmarks/regression queries"
go test -v -tags standalone $SAVEDIR $QUIET || FOUND_DIFFS=1

if [[ $CLEANUP == all ]]; then
    Info "bringing down zero and alpha and data volumes"
    DockerCompose down -v
elif [[ $CLEANUP == none ]]; then
    Info "leaving up zero and alpha"
else
    Info "bringing down zero and alpha only"
    DockerCompose down
fi

if [[ $FOUND_DIFFS -eq 0 ]]; then
    Info "no diffs found in query results"
else
    Info "found some diffs in query results"
fi

exit $FOUND_DIFFS
