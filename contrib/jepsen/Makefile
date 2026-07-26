#
# SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
# SPDX-License-Identifier: Apache-2.0
#

DIR="../../dgraph"

.PHONY: install

install: jepsen
	$(MAKE) -C $(DIR) install;

jepsen: main.go
	go build -v .

