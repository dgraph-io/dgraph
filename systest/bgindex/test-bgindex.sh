#!/bin/bash

set -e
readonly SRCDIR=$(dirname $0)

function Info {
    echo -e "INFO: $*"
}

function DockerCompose {
    docker-compose -p dgraph "$@"
}

Info "entering directory $SRCDIR"
cd $SRCDIR

Info "bringing down dgraph cluster and data volumes"
DockerCompose down -v --remove-orphans

Info "bringing up dgraph cluster"
DockerCompose up -d --remove-orphans

Info "waiting for zero to become leader"
DockerCompose logs -f alpha1 | grep -q -m1 "Successfully upserted groot account"

if [[ ! -z "$TEAMCITY_VERSION" ]]; then
    # Make TeamCity aware of Go tests
    export GOFLAGS="-json"
fi

Info "running background indexing test"
go test -v -tags systest || FOUND_DIFFS=1

Info "bringing down dgraph cluster and data volumes"
DockerCompose down -v --remove-orphans

if [[ $FOUND_DIFFS -eq 0 ]]; then
    Info "test passed"
else
    Info "test failed"
fi

exit $FOUND_DIFFS
