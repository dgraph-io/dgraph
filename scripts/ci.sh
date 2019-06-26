#!/bin/bash

echo ">> Running tests..."
go test -v -short -coverprofile c.out ./...
./cc-test-reporter after-build --exit-code $?
