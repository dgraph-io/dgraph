#!/bin/bash
#

if [[ $1 == "-h" || $1 == "--help" ]]; then
    cat <<EOF
usage: ./run.sh

description:

    Rebuild dgraph and bring up the docker-compose config found here.

    The docker-compose.yml file to bring up must be created with ./compose 
    before running run.sh.
EOF
    exit 0
fi

function Info {
    echo -e "INFO: $*"
}

Info "rebuilding dgraph ..."
( cd ../dgraph ; make install )

Info "bringing up containers"
docker-compose -p dgraph up --force-recreate --remove-orphans
