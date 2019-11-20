#!/bin/bash

dir=$(dirname "${BASH_SOURCE[0]}")
pushd $dir
set -e
make test
popd
