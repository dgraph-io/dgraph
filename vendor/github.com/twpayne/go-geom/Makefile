goversion=$(word 3,$(shell go version))
SRC=$(shell find . -name \*.go)

all: build committed

build:
	go test -v ./...
	if [ "${goversion}" = "go1.8" ]; then \
		go vet ./... ; \
		go get github.com/golang/lint/golint ; \
		golint ./... ; \
		go get golang.org/x/tools/cmd/goimports ; \
		! goimports -l ${SRC} 2>&1 | read ; \
		go test -cover -race ./... ; \
	fi
	go generate ./...

committed:
	git diff --exit-code

