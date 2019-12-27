#!/usr/bin/env bash

PROJECTNAME=$(shell basename "$(PWD)")
GOLANGCI := $(GOPATH)/bin/golangci-lint
COMPANY=chainsafe
NAME=gossamer
VERSION=latest
FULLDOCKERNAME=$(COMPANY)/$(NAME):$(VERSION)

.PHONY: help lint test install build clean start docker gossamer
all: help
help: Makefile
	@echo
	@echo " Choose a make command to run in "$(PROJECTNAME)":"
	@echo
	@sed -n 's/^##//p' $< | column -t -s ':' |  sed -e 's/^/ /'
	@echo

$(GOLANGCI):
	wget -O - -q https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh -s latest

## lint: Lints project files, go gets golangci-lint if missing. Runs `golangci-lint` on project files.
.PHONY: lint
lint: $(GOLANGCI)
	golangci-lint run ./... -c .golangci.yml

clean:
	rm -fr ./build

format:
	./scripts/goimports.sh

## test: Runs `go test` on project test files.
test:
	@echo "  >  \033[32mRunning tests...\033[0m "
	go test ./... -v

## install: Install missing dependencies. Runs `go mod download` internally.
install:
	@echo "  >  \033[32mInstalling dependencies...\033[0m "
	go mod download

## build: Builds application binary and stores it in `./bin/gossamer`
build:
	@echo "  >  \033[32mBuilding binary...\033[0m "
	GOBIN=$(PWD)/build/bin go run scripts/ci.go install

## start: Starts application from binary executable in `./bin/gossamer`
start:
	@echo "  >  \033[32mStarting server...\033[0m "
	./build/bin/gossamer

$(ADDLICENSE):
	go get -u github.com/google/addlicense

## license: Adds license header to missing files, go gets addLicense if missing. Runs `addlicense -c gossamer -f ./copyright.txt -y 2019 .` on project files.
.PHONY: license
license: $(ADDLICENSE)
	@echo "  >  \033[32mAdding license headers...\033[0m "
	addlicense -c gossamer -f ./copyright.txt -y 2019 .

docker: docker-build
	@echo "  >  \033[32mStarting Gossamer Container...\033[0m "
	docker run --rm $(FULLDOCKERNAME)

docker-build:
	@echo "  >  \033[32mBuilding Docker Container...\033[0m "
	docker build -t $(FULLDOCKERNAME) -f Dockerfile.dev .

gossamer: clean
	GOBIN=$(PWD)/build/bin go run scripts/ci.go install
