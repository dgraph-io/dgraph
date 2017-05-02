#!/bin/bash

set -e

pushd contrib/releases &> /dev/null

# Building embedded binaries.
./build.sh

ls
popd &> /dev/null
