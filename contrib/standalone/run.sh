#!/bin/bash

# fail if any error occurs
set -e

echo -e "\033[0;33m
Warning: This standalone version is meant for quickstart purposes only.
         It is NOT RECOMMENDED for production environments.\033[0;0m"

# For Dgraph versions v20.11 and older
export DGRAPH_ALPHA_WHITELIST=0.0.0.0/0
# For Dgraph versions v21.03 and newer
export DGRAPH_ALPHA_SECURITY='whitelist=0.0.0.0/0'

# TODO properly handle SIGTERM for all three processes.
dgraph-ratel & dgraph zero & dgraph alpha
