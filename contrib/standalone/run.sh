#!/bin/bash

# fail if any error occurs
set -e

lru_kb=$(cat /proc/meminfo | grep MemTotal | sed "s/.* \([0-9]*\) .*/\1/")
lru_mb=$(expr $lru_kb / 1024)
echo "running alpha with LRU size of $lru_mb MB"
dgraph-ratel & dgraph zero & dgraph alpha --lru_mb $lru_mb