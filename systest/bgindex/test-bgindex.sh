#!/bin/bash

set -e

function Info {
    echo -e "INFO: $*"
}

function DockerCompose {
    docker-compose -p dgraph "$@"
}

Info "bringing down dgraph cluster and data volumes"
DockerCompose down -v

Info "bringing up dgraph cluster"
DockerCompose up -d

Info "waiting for zero to become leader"
DockerCompose logs -f zero1 | grep -q -m1 "I've become the leader"

if [[ ! -z "$TEAMCITY_VERSION" ]]; then
    # Make TeamCity aware of Go tests
    export GOFLAGS="-json"
fi

Info "running background indexing test"
go test -v -tags systest || FOUND_DIFFS=1

Info "bringing down dgraph cluster and data volumes"
DockerCompose down -v

if [[ $FOUND_DIFFS -eq 0 ]]; then
    Info "test passed"
else
    Info "test failed"
fi

exit $FOUND_DIFFS
