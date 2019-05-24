PROJECTNAME=$(shell basename "$(PWD)")
GOLANGCI := $(GOPATH)/bin/golangci-lint

.PHONY: help
all: help
help: Makefile
	@echo
	@echo " Choose a make command to run in "$(PROJECTNAME)":"
	@echo
	@sed -n 's/^##//p' $< | column -t -s ':' |  sed -e 's/^/ /'
	@echo

$(GOLANGCI):
	curl -sfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh -s latest

## lint: Lints project files, go gets golangci-lint if missing. Runs `golangci-lint` on project files.
.PHONY: lint
lint: $(GOLANGCI)
	golangci-lint run -v

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
	go build -o ./bin/gossamer

## start: Starts application from binary executable in `./bin/gossamer`
start:
	@echo "  >  \033[32mStarting server...\033[0m "
	./bin/gossamer

docker:
	@echo "  >  \033[32mBuilding Docker Container...\033[0m "
	docker build -t chainsafe/gossamer -f Dockerfile.dev .
	@echo "  >  \033[32mRunning Docker Container...\033[0m "
	docker run chainsafe/gossamer

gossamer:
	cd cmd/gossamer && go install
