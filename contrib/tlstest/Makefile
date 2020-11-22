#
# Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#


DGRAPH_PATH = $(GOPATH)/src/github.com/dgraph-io/dgraph/dgraph
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

