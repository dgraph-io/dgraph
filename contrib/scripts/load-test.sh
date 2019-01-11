#!/bin/bash

set -e

total_mem_kb=`cat /proc/meminfo | awk '/MemTotal:/ {print $2}'`
if [[ $total_mem_kb -lt 32000000 ]]; then
    echo >&2 "Load test requires system with at least 32GB of memory"
    exit 1
fi

bash contrib/scripts/loader.sh $1
bash contrib/scripts/transactions.sh $1
