#
# SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
# SPDX-License-Identifier: Apache-2.0
#

DIR="../../dgraph"

.PHONY: install

install: jepsen
	$(MAKE) -C $(DIR) install;

jepsen: main.go
	go build -v .

