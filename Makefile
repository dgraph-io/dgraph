PKGS := $(shell go list ./... | grep -v /vendor)

PROJECTNAME=$(shell basename "$(PWD)")
GOBASE=$(shell pwd)
GOBIN=$(GOBASE)/bin

BIN_DIR := $(GOPATH)/bin
GOMETALINTER := $(BIN_DIR)/gometalinter

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

$(GOMETALINTER):
	go get -u github.com/alecthomas/gometalinter
	gometalinter --install &> /dev/null

## lint: Lints project files, go gets gometalinter if missing. Runs `gometalinter` on project files.
.PHONY: lint
lint: $(GOMETALINTER)
	gometalinter ./... --vendor

## install: Install missing dependencies. Runs `go get` internally.
install:
	@echo "  >  \033[32mInstalling dependencies...\033[0m "
	@GOPATH=$(GOPATH) GOBIN=$(GOBIN) go get $(get)

## clean: Clean build files. Runs `go clean` internally.
clean:
	@echo "  >  \033[32mCleaning build cache...\033[0m "
	@GOPATH=$(GOPATH) GOBIN=$(GOBIN) go clean