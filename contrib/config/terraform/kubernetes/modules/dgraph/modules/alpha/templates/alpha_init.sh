#!/usr/bin/env bash

trap "exit" SIGINT SIGTERM
echo "Write to /dgraph/doneinit when ready."
until [ -f /dgraph/doneinit ]; do sleep 2; done
