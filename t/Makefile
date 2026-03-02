#
# SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
# SPDX-License-Identifier: Apache-2.0
#

# OS detection (lowercase: linux, darwin)
OS := $(shell uname -s | tr '[:upper:]' '[:lower:]')

# linux || darwin
GOOS          ?= $(shell go env GOOS)
export GOPATH ?= $(shell go env GOPATH)
GOHOSTOS      := $(shell go env GOHOSTOS)
GOHOSTARCH    := $(shell go env GOHOSTARCH)

# On non-Linux systems, use a separate directory for Linux binaries
ifeq ($(GOHOSTOS),linux)
    export LINUX_GOBIN ?= $(GOPATH)/bin
else
    export LINUX_GOBIN ?= $(GOPATH)/linux_$(GOHOSTARCH)
endif

all: test

# ===========================================================================
# Dependency checking
# ===========================================================================

.PHONY: check-deps
check-deps: check-deps-go check-deps-docker check-deps-gotestsum check-deps-ack check-deps-cross-compiler check-deps-protoc check-docker-available-memory
	@echo "All dependencies are installed"

.PHONY: check
check: check-deps
	@echo "LINUX_GOBIN=$(LINUX_GOBIN)"
	@if [ -f "$(LINUX_GOBIN)/dgraph" ]; then \
		file $(LINUX_GOBIN)/dgraph | grep -q "ELF.*executable" || (echo "Error: dgraph binary at $(LINUX_GOBIN)/dgraph is not a Linux executable" && exit 1); \
	else \
		echo "Error: dgraph binary not found at $(LINUX_GOBIN)/dgraph" && exit 1; \
	fi
	@echo "The dgraph binary is a Linux executable (as required)"

# ---- check-deps-{tool} targets ----
#
# Flow for each tool:
#   1. Silent check → pass → done
#   2. Silent check → fail + AUTO_INSTALL=true → install via OS target → re-check
#   3. Silent check → fail + no AUTO_INSTALL → re-run with output → exit 1

.PHONY: check-deps-go
check-deps-go:
	@if AUTO_INSTALL= ./scripts/check-deps-go.sh >/dev/null 2>&1; then \
		:; \
	elif [ "$(AUTO_INSTALL)" = "true" ]; then \
		$(MAKE) install-deps-go-$(OS) && \
		AUTO_INSTALL= ./scripts/check-deps-go.sh; \
	else \
		AUTO_INSTALL= ./scripts/check-deps-go.sh || exit 1; \
	fi

.PHONY: check-deps-docker
check-deps-docker:
	@if AUTO_INSTALL= ./scripts/check-deps-docker.sh >/dev/null 2>&1; then \
		:; \
	elif [ "$(AUTO_INSTALL)" = "true" ]; then \
		$(MAKE) install-deps-docker-$(OS) && \
		AUTO_INSTALL= ./scripts/check-deps-docker.sh; \
	else \
		AUTO_INSTALL= ./scripts/check-deps-docker.sh || exit 1; \
	fi

.PHONY: check-deps-gotestsum
check-deps-gotestsum:
	@if AUTO_INSTALL= ./scripts/check-deps-gotestsum.sh >/dev/null 2>&1; then \
		:; \
	elif [ "$(AUTO_INSTALL)" = "true" ]; then \
		$(MAKE) install-deps-gotestsum-$(OS) && \
		AUTO_INSTALL= ./scripts/check-deps-gotestsum.sh; \
	else \
		AUTO_INSTALL= ./scripts/check-deps-gotestsum.sh || exit 1; \
	fi

.PHONY: check-deps-ack
check-deps-ack:
	@if AUTO_INSTALL= ./scripts/check-deps-ack.sh >/dev/null 2>&1; then \
		:; \
	elif [ "$(AUTO_INSTALL)" = "true" ]; then \
		$(MAKE) install-deps-ack-$(OS) && \
		AUTO_INSTALL= ./scripts/check-deps-ack.sh; \
	else \
		AUTO_INSTALL= ./scripts/check-deps-ack.sh || exit 1; \
	fi

.PHONY: check-deps-cross-compiler
check-deps-cross-compiler:
	@if AUTO_INSTALL= ./scripts/check-deps-cross-compiler.sh >/dev/null 2>&1; then \
		:; \
	elif [ "$(AUTO_INSTALL)" = "true" ]; then \
		$(MAKE) install-deps-cross-compiler-$(OS) && \
		AUTO_INSTALL= ./scripts/check-deps-cross-compiler.sh; \
	else \
		AUTO_INSTALL= ./scripts/check-deps-cross-compiler.sh || exit 1; \
	fi

.PHONY: check-deps-protoc
check-deps-protoc:
	@if [ "$(OS)" = "linux" ]; then \
		if AUTO_INSTALL= ./scripts/check-deps-protoc.sh >/dev/null 2>&1; then :; \
		elif [ "$(AUTO_INSTALL)" = "true" ]; then \
			$(MAKE) install-deps-protoc-$(OS) && \
			AUTO_INSTALL= ./scripts/check-deps-protoc.sh; \
		else \
			AUTO_INSTALL= ./scripts/check-deps-protoc.sh || exit 1; \
		fi; \
	fi

# ---- Docker memory check ----

.PHONY: check-docker-available-memory
check-docker-available-memory: check-deps-docker
	@$(MAKE) check-docker-available-memory-$(OS)

.PHONY: check-docker-available-memory-darwin
check-docker-available-memory-darwin:
	@./scripts/check-docker-available-memory.sh

.PHONY: check-docker-available-memory-linux
check-docker-available-memory-linux:
	@./scripts/check-docker-available-memory.sh

# ===========================================================================
# Install targets — Homebrew prerequisite (darwin)
# ===========================================================================

.PHONY: check-install-deps-homebrew-darwin
check-install-deps-homebrew-darwin:
	@[ "$(OS)" = "darwin" ] || { echo "ERROR: Homebrew is only available on macOS"; exit 1; }
	@AUTO_INSTALL=$(AUTO_INSTALL) ./scripts/check-deps-brew.sh

# ===========================================================================
# Install targets — darwin
# ===========================================================================

.PHONY: install-deps-go-darwin
install-deps-go-darwin: check-install-deps-homebrew-darwin
	@brew upgrade go 2>/dev/null || brew install go

.PHONY: install-deps-docker-darwin
install-deps-docker-darwin: check-install-deps-homebrew-darwin
	@brew install --cask docker

.PHONY: install-deps-gotestsum-darwin
install-deps-gotestsum-darwin: check-install-deps-homebrew-darwin
	@go install gotest.tools/gotestsum@latest

.PHONY: install-deps-ack-darwin
install-deps-ack-darwin: check-install-deps-homebrew-darwin
	@brew install ack

.PHONY: install-deps-cross-compiler-darwin
install-deps-cross-compiler-darwin: check-install-deps-homebrew-darwin
	@brew tap messense/macos-cross-toolchains && \
	brew install messense/macos-cross-toolchains/$$(case $$(uname -m) in arm64|aarch64) echo aarch64-unknown-linux-gnu;; x86_64) echo x86_64-unknown-linux-gnu;; esac)

.PHONY: install-deps-protoc-darwin
install-deps-protoc-darwin: check-install-deps-homebrew-darwin
	@brew install protobuf

# ===========================================================================
# Install targets — linux
# ===========================================================================

.PHONY: install-deps-go-linux
install-deps-go-linux:
	@echo ""; \
	echo "Go is not installed or is below the required version."; \
	echo ""; \
	VERSION=$$(grep -E '^go [0-9]+\.[0-9]+' ../go.mod | awk '{print $$2}'); \
	echo "Required version: Go $${VERSION} (from go.mod)"; \
	echo ""; \
	echo "Install or upgrade Go using the official instructions:"; \
	echo "  https://go.dev/doc/install"; \
	echo ""; \
	ARCH=$$(uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64/'); \
	echo "Quick steps for Linux:"; \
	echo "  curl -fsSLO https://go.dev/dl/go$${VERSION}.linux-$${ARCH}.tar.gz"; \
	echo "  sudo rm -rf /usr/local/go"; \
	echo "  sudo tar -C /usr/local -xzf go$${VERSION}.linux-$${ARCH}.tar.gz"; \
	echo "  export PATH=\$$PATH:/usr/local/go/bin"; \
	echo ""; \
	exit 1

.PHONY: install-deps-docker-linux
install-deps-docker-linux:
	@if command -v apt-get >/dev/null 2>&1; then $(MAKE) install-deps-docker-linux-apt; \
	elif command -v dnf >/dev/null 2>&1; then $(MAKE) install-deps-docker-linux-dnf; \
	elif command -v pacman >/dev/null 2>&1; then $(MAKE) install-deps-docker-linux-pacman; \
	else echo "ERROR: No supported package manager found (tried: apt, dnf, pacman)"; exit 1; fi

.PHONY: install-deps-docker-linux-apt
install-deps-docker-linux-apt:
	@sudo apt-get update && sudo apt-get install -y ca-certificates curl gnupg && \
	sudo install -m 0755 -d /etc/apt/keyrings && \
	curl -fsSL https://download.docker.com/linux/$$(. /etc/os-release && echo "$${ID:-ubuntu}")/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg && \
	sudo chmod a+r /etc/apt/keyrings/docker.gpg && \
	echo "deb [arch=$$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/$$(. /etc/os-release && echo "$${ID:-ubuntu}") $$(. /etc/os-release && echo "$${VERSION_CODENAME}") stable" | sudo tee /etc/apt/sources.list.d/docker.list >/dev/null && \
	sudo apt-get update && \
	sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

.PHONY: install-deps-docker-linux-dnf
install-deps-docker-linux-dnf:
	@sudo dnf -y install dnf-plugins-core && \
	sudo dnf config-manager --add-repo https://download.docker.com/linux/fedora/docker-ce.repo && \
	sudo dnf install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

.PHONY: install-deps-docker-linux-pacman
install-deps-docker-linux-pacman:
	@sudo pacman -S --noconfirm docker docker-compose

.PHONY: install-deps-gotestsum-linux
install-deps-gotestsum-linux:
	@go install gotest.tools/gotestsum@latest

.PHONY: install-deps-ack-linux
install-deps-ack-linux:
	@if command -v apt-get >/dev/null 2>&1; then sudo apt-get update && sudo apt-get install -y ack; \
	elif command -v dnf >/dev/null 2>&1; then sudo dnf install -y ack; \
	elif command -v pacman >/dev/null 2>&1; then sudo pacman -S --noconfirm ack; \
	else echo "ERROR: No supported package manager found (tried: apt, dnf, pacman)"; exit 1; fi

.PHONY: install-deps-cross-compiler-linux
install-deps-cross-compiler-linux:
	@: # Cross-compiler not needed on Linux

.PHONY: install-deps-protoc-linux
install-deps-protoc-linux:
	@if command -v apt-get >/dev/null 2>&1; then sudo apt-get update && sudo apt-get install -y protobuf-compiler; \
	elif command -v dnf >/dev/null 2>&1; then sudo dnf install -y protobuf-compiler; \
	elif command -v pacman >/dev/null 2>&1; then sudo pacman -S --noconfirm protobuf; \
	else echo "ERROR: No supported package manager found (tried: apt, dnf, pacman)"; exit 1; fi

# ===========================================================================
# Build & test
# ===========================================================================

.PHONY: dgraph-installed
dgraph-installed:
	@$(MAKE) -C .. dgraph-installed

.PHONY: test
test: dgraph-installed check
# 	build the t.go binary
	@go build .
#	clean go testcache
	@go clean -testcache
#	run t.go with specified arguments; otherwise run standard suite
	@if [ -n "$(args)" ]; then \
		./t $(args); \
	else \
		./t; \
	fi
#	clean up docker containers after test execution
	@./t -r
