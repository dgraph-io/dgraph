#
# SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
# SPDX-License-Identifier: Apache-2.0
#

BIN           = compose
BUILD_FLAGS  ?= "-N -l"

.PHONY: all
all: install_dgraph $(BIN)

.PHONY: install
install: all

.PHONY: install_dgraph
install_dgraph:
	$(MAKE) -C ../dgraph install

$(BIN): compose.go
	go build -gcflags=$(BUILD_FLAGS) -o $(BIN)

.PHONY: clean
clean:
	rm -f $(BIN) docker-compose*.yml
