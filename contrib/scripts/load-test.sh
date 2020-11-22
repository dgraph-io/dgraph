#!/bin/bash

ONE_GB=$((1024 ** 3))
REQUIRED_MEM=$((20 * ONE_GB))

set -e

total_mem_kb=`cat /proc/meminfo | awk '/MemTotal:/ {print $2}'`
if [[ $total_mem_kb -lt $((REQUIRED_MEM / 1024)) ]]; then
    printf >&2 "Load test requires system with at least %dGB of memory\n" \
                $((REQUIRED_MEM / ONE_GB))
    exit 1
fi

bash contrib/scripts/loader.sh $1
bash contrib/scripts/transactions.sh $1
