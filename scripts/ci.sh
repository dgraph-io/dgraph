#!/bin/bash

echo ">> Running tests..."
go test -short -coverprofile c.out ./...
./cc-test-reporter after-build --exit-code $?
