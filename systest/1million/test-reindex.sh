#!/bin/bash

set -e
readonly ME=${0##*/}
readonly SRCDIR=$(dirname $0)

BENCHMARKS_REPO="$(pwd)/benchmarks"
NO_INDEX_SCHEMA_FILE="$BENCHMARKS_REPO/data/1million-noindex.schema"
SCHEMA_FILE="$BENCHMARKS_REPO/data/1million.schema"
DATA_FILE="$BENCHMARKS_REPO/data/1million.rdf.gz"

function Info {
    echo -e "INFO: $*"
}

function DockerCompose {
    docker-compose -p dgraph "$@"
}

Info "cloning benchmarks repo"
BENCHMARKS_URL=https://github.com/dgraph-io/benchmarks/blob/master/data
rm -rf $BENCHMARKS_REPO
mkdir -p $BENCHMARKS_REPO/data
wget -O $BENCHMARKS_REPO/data/1million.schema $BENCHMARKS_URL/1million.schema?raw=true
wget -O $BENCHMARKS_REPO/data/1million-noindex.schema $BENCHMARKS_URL/1million-noindex.schema?raw=true
wget -O $BENCHMARKS_REPO/data/1million.rdf.gz $BENCHMARKS_URL/1million.rdf.gz?raw=true

Info "bringing down zero and alpha and data volumes"
DockerCompose down -v

Info "bringing up zero container"
DockerCompose up -d --remove-orphans --force-recreate zero1

Info "waiting for zero to become leader"
DockerCompose logs -f zero1 | grep -q -m1 "I've become the leader"

Info "bulk loading data set"
DockerCompose run -v $BENCHMARKS_REPO:$BENCHMARKS_REPO --name bulk_load zero1 \
    bash -s <<EOF
        mkdir -p /data/alpha1
        mkdir -p /data/alpha2
        mkdir -p /data/alpha3
        /gobin/dgraph bulk --schema=$NO_INDEX_SCHEMA_FILE --files=$DATA_FILE \
                            --format=rdf --zero=zero1:5180 --out=/data/zero1/bulk \
                            --reduce_shards 3 --map_shards 9
        mv /data/zero1/bulk/0/p /data/alpha1
        mv /data/zero1/bulk/1/p /data/alpha2
        mv /data/zero1/bulk/2/p /data/alpha3
EOF

Info "bringing up alpha container"
DockerCompose up -d --force-recreate alpha1 alpha2 alpha3

Info "waiting for alpha to be ready"
DockerCompose logs -f alpha1 | grep -q -m1 "Server is ready"
# after the server prints the log "Server is ready", it may be still loading data from badger
Info "sleeping for 5 seconds for the server to be ready"
sleep 5

Info "building indexes"
curl localhost:8180/alter --data-binary @$SCHEMA_FILE

if [[ ! -z "$TEAMCITY_VERSION" ]]; then
    # Make TeamCity aware of Go tests
    export GOFLAGS="-json"
fi

Info "running regression queries"
go test -v || FOUND_DIFFS=1

Info "bringing down zero and alpha and data volumes"
DockerCompose down -v

if [[ $FOUND_DIFFS -eq 0 ]]; then
    Info "no diffs found in query results"
else
    Info "found some diffs in query results"
fi

rm -rf $BENCHMARKS_REPO
exit $FOUND_DIFFS
