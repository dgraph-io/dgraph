package runtime

import (
	"path/filepath"
	"testing"

	"github.com/ChainSafe/gossamer/keystore"
	"github.com/ChainSafe/gossamer/tests"
	"github.com/ChainSafe/gossamer/trie"
	"github.com/stretchr/testify/require"
)

// NewTestRuntime will create a new runtime (polkadot/test)
func NewTestRuntime(t *testing.T, targetRuntime string) *Runtime {
	return NewTestRuntimeWithTrie(t, targetRuntime, nil)
}

// NewTestRuntimeWithTrie will create a new runtime (polkadot/test) with the supplied trie as the storage
func NewTestRuntimeWithTrie(t *testing.T, targetRuntime string, tt *trie.Trie) *Runtime {
	testRuntimeFilePath, testRuntimeURL := tests.GetRuntimeVars(targetRuntime)

	_, err := tests.GetRuntimeBlob(testRuntimeFilePath, testRuntimeURL)
	require.Nil(t, err, "Fail: could not get runtime", "targetRuntime", targetRuntime)

	rs := tests.NewTestRuntimeStorage(tt)

	fp, err := filepath.Abs(testRuntimeFilePath)
	require.Nil(t, err, "could not create testRuntimeFilePath", "targetRuntime", targetRuntime)

	r, err := NewRuntimeFromFile(fp, rs, keystore.NewKeystore())
	require.Nil(t, err, "Got error when trying to create new VM", "targetRuntime", targetRuntime)
	require.NotNil(t, r, "Could not create new VM instance", "targetRuntime", targetRuntime)

	return r
}
