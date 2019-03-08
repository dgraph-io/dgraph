PKGS := $(shell go list ./... | grep -v /vendor)

PROJECTNAME=$(shell basename "$(PWD)")
GOBASE=$(shell pwd)
GOBIN=$(GOBASE)/bin

BIN_DIR := $(GOPATH)/bin
GOLANGCI-LINT := $(BIN_DIR)/golangci

.PHONY: help
all: help
help: Makefile
	@echo
	@echo " Choose a make command to run in "$(PROJECTNAME)":"
	@echo
	@sed -n 's/^##//p' $< | column -t -s ':' |  sed -e 's/^/ /'
	@echo

## test: Runs `go test` on project test files.
.PHONY: test
test:
	go test $(PKGS)

$(GOLANGCI-LINT):
	go get -u github.com/golangci/golangci-lint/cmd/golangci-lint

## lint: Lints project files, go gets golangci-lint if missing. Runs `golangci-lint run` on project files.
.PHONY: lint
lint: $(GOLANGCI-LINT)
	golangci-lint run ./... --enable gofmt --enable goimports

## install: Install missing dependencies. Runs `go get` internally.
install:
	@echo "  >  \033[32mInstalling dependencies...\033[0m "
	@GOPATH=$(GOPATH) GOBIN=$(GOBIN) go get $(get)

## clean: Clean build files. Runs `go clean` internally.
clean:
	@echo "  >  \033[32mCleaning build cache...\033[0m "
	@GOPATH=$(GOPATH) GOBIN=$(GOBIN) go clean
