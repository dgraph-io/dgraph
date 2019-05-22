#!/bin/bash
# Builds ./embargo and runs the embargo tests.
#
# Usage:
# Run the test 32 times (about 8 hours):
#     ./run.sh
# Run the test once:
#     ./run.sh 1

function cleanup_embargo {
    embargo destroy || true
    docker container prune -f
    if docker network ls | grep -q 'embargo_net'; then
        docker network ls |
            awk '/embargo_net/ { print $1 }' |
            xargs docker network rm
    fi
}


set -x -o pipefail

times=${1:-32}

go build -v .

cleanup_embargo
# Each run takes about 15 minutes, so running 32 times will take about 8 hours.
for i in $(seq 1 $times)
do
    echo "===> Running Embargo #$i"
    if ! ./embargo 2>&1 | tee embargo$i.log; then
        echo "===> Embargo test failed"
        docker logs zero1 2>&1 | tee zero1.log
        docker logs zero2 2>&1 | tee zero2.log
        docker logs zero3 2>&1 | tee zero3.log
        docker logs dg1 2>&1 | tee dg1.log
        docker logs dg2 2>&1 | tee dg2.log
        docker logs dg3 2>&1 | tee dg3.log

        cleanup_embargo
        exit 1
    fi
done

echo "Embargo log summary:"
grep '===>' embargo*.log

cleanup_embargo
