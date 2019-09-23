#!/bin/bash

# fail if any error occurs
set -e

lru_kb=$(cat /proc/meminfo | grep MemTotal | sed "s/.* \([0-9]*\) .*/\1/")
lru_mb=$(expr $lru_kb / 1024 / 3) # one-third of host memory

echo -e "\033[0;33m
Warning: This standalone version is meant for quickstart purposes only.
         It is NOT RECOMMENDED for production environments.\033[0;0m"

# TODO properly handle SIGTERM for all three processes.
dgraph-ratel & dgraph zero & dgraph alpha --lru_mb $lru_mb
