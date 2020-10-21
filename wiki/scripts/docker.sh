#!/bin/bash
cd "$(dirname "$0")" || exit

# Note: Never start this without committing the changes. As this will checkout between several branches. Any staged change can bring issues.

rm -rf ../public

export HUGO_CANONIFYURLS=false
# if you are using Docker in a VM, please change the HOST value to the machine\'s IP.
HOST="http://localhost/docs" \
    PUBLIC="./public/docs" \
    LOOP=false \
    ./build.sh

cd ..
docker-compose up --remove-orphans --force-recreate
