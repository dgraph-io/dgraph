package tests

import (
	"io"
	"net/http"
	"os"
	"path"

	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/trie"
)

const (
	POLKADOT_RUNTIME     = "polkadot_runtime"
	POLKADOT_RUNTIME_FP  = "../substrate_test_runtime.compact.wasm"
	POLKADOT_RUNTIME_URL = "https://github.com/noot/substrate/blob/add-blob/core/test-runtime/wasm/wasm32-unknown-unknown/release/wbuild/substrate-test-runtime/substrate_test_runtime.compact.wasm?raw=true"

	TEST_RUNTIME  = "test_runtime"
	TESTS_FP      = "test_wasm.wasm"
	TEST_WASM_URL = "https://github.com/ChainSafe/gossamer-test-wasm/blob/noot/target/wasm32-unknown-unknown/release/test_wasm.wasm?raw=true"

	SIMPLE_WASM_FP     = "simple.wasm"
	SIMPLE_RUNTIME_URL = "https://github.com//wasmerio/go-ext-wasm/blob/master/wasmer/test/testdata/examples/simple.wasm?raw=true"
)

// GetAbsolutePath returns the completePath for a given targetDir
func GetAbsolutePath(targetDir string) string {
	dir, err := os.Getwd()
	if err != nil {
		panic("Could not get current dir for test!")
	}
	completePath := path.Join(dir, targetDir)

	return completePath
}

// GetRuntimeVars returns the testRuntimeFilePath and testRuntimeURL
func GetRuntimeVars(targetRuntime string) (string, string) {
	testRuntimeFilePath, testRuntimeURL := GetAbsolutePath(TESTS_FP), TEST_WASM_URL

	// If target runtime is polkadot, re-assign vars
	if targetRuntime == POLKADOT_RUNTIME {
		testRuntimeFilePath, testRuntimeURL = GetAbsolutePath(POLKADOT_RUNTIME_FP), POLKADOT_RUNTIME_URL
	}
	return testRuntimeFilePath, testRuntimeURL
}

// GetRuntimeBlob checks if the test wasm @testRuntimeFilePath exists and if not, it fetches it from @testRuntimeURL
func GetRuntimeBlob(testRuntimeFilePath, testRuntimeURL string) (n int64, err error) {
	if Exists(testRuntimeFilePath) {
		return 0, nil
	}

	out, err := os.Create(testRuntimeFilePath)
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = out.Close()
	}()

	/* #nosec */
	resp, err := http.Get(testRuntimeURL)
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	n, err = io.Copy(out, resp.Body)
	return n, err
}

// Exists reports whether the named file or directory exists.
func Exists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// TestRuntimeStorage holds trie pointer
type TestRuntimeStorage struct {
	trie *trie.Trie
}

// NewTestRuntimeStorage creates new instance of TestRuntimeStorage
func NewTestRuntimeStorage(tr *trie.Trie) *TestRuntimeStorage {
	if tr == nil {
		tr = trie.NewEmptyTrie(nil)
	}
	return &TestRuntimeStorage{
		trie: tr,
	}
}

// TrieAsString is a dummy test func
func (trs TestRuntimeStorage) TrieAsString() string {
	return trs.trie.String()
}

// SetStorage is a dummy test func
func (trs TestRuntimeStorage) SetStorage(key []byte, value []byte) error {
	return trs.trie.Put(key, value)
}

// GetStorage is a dummy test func
func (trs TestRuntimeStorage) GetStorage(key []byte) ([]byte, error) {
	return trs.trie.Get(key)
}

// StorageRoot is a dummy test func
func (trs TestRuntimeStorage) StorageRoot() (common.Hash, error) {
	return trs.trie.Hash()
}

// SetStorageChild is a dummy test func
func (trs TestRuntimeStorage) SetStorageChild(keyToChild []byte, child *trie.Trie) error {
	return trs.trie.PutChild(keyToChild, child)
}

// SetStorageIntoChild is a dummy test func
func (trs TestRuntimeStorage) SetStorageIntoChild(keyToChild, key, value []byte) error {
	return trs.trie.PutIntoChild(keyToChild, key, value)
}

// GetStorageFromChild is a dummy test func
func (trs TestRuntimeStorage) GetStorageFromChild(keyToChild, key []byte) ([]byte, error) {
	return trs.trie.GetFromChild(keyToChild, key)
}

// ClearStorage is a dummy test func
func (trs TestRuntimeStorage) ClearStorage(key []byte) error {
	return trs.trie.Delete(key)
}

// Entries is a dummy test func
func (trs TestRuntimeStorage) Entries() map[string][]byte {
	return trs.trie.Entries()
}
