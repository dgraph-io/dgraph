// Copyright 2019 ChainSafe Systems (ON) Corp.
// This file is part of gossamer.
//
// The gossamer library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The gossamer library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the gossamer library. If not, see <http://www.gnu.org/licenses/>.

package runtime

//nolint
const (
	SUBSTRATE_TEST_RUNTIME     = "substrate_test_runtime"
	SUBSTRATE_TEST_RUNTIME_FP  = "substrate_test_runtime.compact.wasm"
	SUBSTRATE_TEST_RUNTIME_URL = "https://github.com/noot/substrate/blob/add-blob-042920/target/wasm32-unknown-unknown/release/wbuild/substrate-test-runtime/substrate_test_runtime.compact.wasm?raw=true"

	NODE_RUNTIME     = "node_runtime"
	NODE_RUNTIME_FP  = "node_runtime.compact.wasm"
	NODE_RUNTIME_URL = "https://github.com/noot/substrate/blob/noot/legacy/target/wasm32-unknown-unknown/release/wbuild/node-runtime/node_runtime.compact.wasm?raw=true"

	TEST_RUNTIME  = "test_runtime"
	TESTS_FP      = "test_wasm.wasm"
	TEST_WASM_URL = "https://github.com/ChainSafe/gossamer-test-wasm/blob/noot/target/wasm32-unknown-unknown/release/test_wasm.wasm?raw=true"

	SIMPLE_WASM_FP     = "simple.wasm"
	SIMPLE_RUNTIME_URL = "https://github.com//wasmerio/go-ext-wasm/blob/master/wasmer/test/testdata/examples/simple.wasm?raw=true"
)
