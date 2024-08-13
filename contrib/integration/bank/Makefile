.PHONY: build

DOCKER_REPO ?= dgraph

build:
	go build -o bank
	docker build -t $(DOCKER_REPO)/bank:latest .
