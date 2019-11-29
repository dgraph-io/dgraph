#!/bin/sh

find_files() {
  find . ! \( \
      \( \
        -path '.github' \
        -o -path './build' \
      \) -prune \
    \) -name '*.go'
}

GOFMT="gofmt -s -w"
GOIMPORTS="goimports -w"
find_files | xargs $GOIMPORTS
find_files | xargs $GOFMT
