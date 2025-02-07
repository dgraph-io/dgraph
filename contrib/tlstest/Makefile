#
# SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
# SPDX-License-Identifier: Apache-2.0
#

DGRAPH_PATH = $(GOPATH)/src/github.com/hypermodeinc/dgraph/dgraph
DGRAPH_BIN  = $(DGRAPH_PATH)/dgraph

TARGETS     = test1 test2 test3 test4 test5 test6
KEYBITS     = 2048

.PHONY: all
all: cert $(TARGETS)

test: all

cert:
	@echo "Generating CA cert in 'tls' dir."
	@$(MAKE) -C $(DGRAPH_PATH) all
	@$(DGRAPH_BIN) cert --keysize $(KEYBITS) -d $(PWD)/tls -n localhost -c live

test1: cert
	@echo "Test 1: Alpha non-TLS, Live non-TLS"
	@(DGRAPH_BIN=$(DGRAPH_BIN) ./test.sh ./alpha_notls.sh ./live_notls.sh 0)

test2: cert
	@echo "Test 2: Alpha non-TLS, Live TLS"
	@(DGRAPH_BIN=$(DGRAPH_BIN) ./test.sh ./alpha_notls.sh ./live_tls.sh 1)

test3: cert
	@echo "Test 3: Alpha TLS, Live non-TLS"
	@(DGRAPH_BIN=$(DGRAPH_BIN) ./test.sh ./alpha_tls.sh ./live_notls.sh 1)

test4: cert
	@echo "Test 4: Alpha TLS, Live TLS"
	@(DGRAPH_BIN=$(DGRAPH_BIN) ./test.sh ./alpha_tls.sh ./live_tls.sh 0)

test5: cert
	@echo "Test 5: Alpha TLS Auth, Live TLS"
	@(DGRAPH_BIN=$(DGRAPH_BIN) ./test.sh ./alpha_tls_auth.sh ./live_tls_auth.sh 0)

test6: cert
	@echo "Test 6: Alpha TLS reload, Live TLS"
	@(DGRAPH_BIN=$(DGRAPH_BIN) RELOAD_TEST=1 ./test.sh ./alpha_tls.sh ./live_tls.sh 1)

clean:
	git clean -d -f

