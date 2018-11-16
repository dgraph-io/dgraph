#!/bin/bash
# Builds ./blockade and runs the blockade tests.
#
# Usage:
# Run the test 32 times (about 8 hours):
#     ./run.sh
# Run the test once:
#     ./run.sh 1

function cleanup_blockade {
    blockade destroy || true
    docker container prune -f
    if grep 'blockade_net' $(docker network ls); then
        docker network ls |
            awk '/blockade_net/ { print $1 }'
        xargs docker network rm
    fi
}


set -x -o pipefail

times=${1:-32}

go build -v .

cleanup_blockade
# Each run takes about 15 minutes, so running 32 times will take about 8 hours.
for i in $(seq 1 $times)
do
    echo "===> Running Blockade #$i"
    if ! ./blockade 2>&1 | tee blockade$i.log; then
        echo "===> Blockade test failed"
        docker logs zero1 2>&1 | tee zero1.log
        docker logs zero2 2>&1 | tee zero2.log
        docker logs zero3 2>&1 | tee zero3.log
        docker logs dg1 2>&1 | tee dg1.log
        docker logs dg2 2>&1 | tee dg2.log
        docker logs dg3 2>&1 | tee dg3.log

        cleanup_blockade
        exit 1
    fi
done

echo "Blockade log summary:"
grep '===>' blockade*.log

cleanup_blockade
