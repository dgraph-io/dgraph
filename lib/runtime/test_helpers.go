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

import (
	"encoding/binary"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"testing"

	"github.com/ChainSafe/chaindb"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/trie"
	"github.com/ChainSafe/gossamer/lib/utils"

	ma "github.com/multiformats/go-multiaddr"
)

// TestAuthorityDataKey is the location of GRANDPA authority data in the storage trie for NODE_RUNTIME
var TestAuthorityDataKey, _ = common.HexToBytes("0x3a6772616e6470615f617574686f726974696573")

// GetRuntimeVars returns the testRuntimeFilePath and testRuntimeURL
func GetRuntimeVars(targetRuntime string) (string, string) {
	var testRuntimeFilePath string
	var testRuntimeURL string

	switch targetRuntime {
	case SUBSTRATE_TEST_RUNTIME:
		testRuntimeFilePath, testRuntimeURL = GetAbsolutePath(SUBSTRATE_TEST_RUNTIME_FP), SUBSTRATE_TEST_RUNTIME_URL
	case NODE_RUNTIME:
		testRuntimeFilePath, testRuntimeURL = GetAbsolutePath(NODE_RUNTIME_FP), NODE_RUNTIME_URL
	case TEST_RUNTIME:
		testRuntimeFilePath, testRuntimeURL = GetAbsolutePath(TESTS_FP), TEST_WASM_URL
	}

	return testRuntimeFilePath, testRuntimeURL
}

// GetAbsolutePath returns the completePath for a given targetDir
func GetAbsolutePath(targetDir string) string {
	dir, err := os.Getwd()
	if err != nil {
		panic("failed to get current working directory")
	}
	return path.Join(dir, targetDir)
}

// GetRuntimeBlob checks if the test wasm @testRuntimeFilePath exists and if not, it fetches it from @testRuntimeURL
func GetRuntimeBlob(testRuntimeFilePath, testRuntimeURL string) (n int64, err error) {
	if utils.PathExists(testRuntimeFilePath) {
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

// TestRuntimeStorage implements the Storage interface
type TestRuntimeStorage struct {
	db   chaindb.Database
	trie *trie.Trie
}

// NewTestRuntimeStorage returns an empty, initialized TestRuntimeStorage
func NewTestRuntimeStorage(t *testing.T, tr *trie.Trie) *TestRuntimeStorage {
	if tr == nil {
		tr = trie.NewEmptyTrie()
	}

	testDatadirPath, _ := ioutil.TempDir("/tmp", "test-datadir-*")
	db, err := chaindb.NewBadgerDB(testDatadirPath)
	if err != nil {
		t.Fatal("failed to create TestRuntimeStorage database")
	}

	t.Cleanup(func() {
		_ = db.Close()
		_ = os.RemoveAll(testDatadirPath)
	})

	return &TestRuntimeStorage{
		db:   db,
		trie: tr,
	}
}

// Set ...
func (trs *TestRuntimeStorage) Set(key []byte, value []byte) error {
	err := trs.db.Put(key, value)
	if err != nil {
		return err
	}

	return trs.trie.Put(key, value)
}

// Get ...
func (trs *TestRuntimeStorage) Get(key []byte) ([]byte, error) {
	if has, _ := trs.db.Has(key); has {
		return trs.db.Get(key)
	}

	return trs.trie.Get(key)
}

// Root ...
func (trs *TestRuntimeStorage) Root() (common.Hash, error) {
	tt := trie.NewEmptyTrie()
	iter := trs.db.NewIterator()

	for iter.Next() {
		err := tt.Put(iter.Key(), iter.Value())
		if err != nil {
			return common.Hash{}, err
		}
	}

	iter.Release()
	return tt.Hash()
}

// SetChild ...
func (trs *TestRuntimeStorage) SetChild(keyToChild []byte, child *trie.Trie) error {
	return trs.trie.PutChild(keyToChild, child)
}

// SetChildStorage ...
func (trs *TestRuntimeStorage) SetChildStorage(keyToChild, key, value []byte) error {
	return trs.trie.PutIntoChild(keyToChild, key, value)
}

// GetChildStorage ...
func (trs *TestRuntimeStorage) GetChildStorage(keyToChild, key []byte) ([]byte, error) {
	return trs.trie.GetFromChild(keyToChild, key)
}

// Delete ...
func (trs *TestRuntimeStorage) Delete(key []byte) error {
	err := trs.db.Del(key)
	if err != nil {
		return err
	}

	return trs.trie.Delete(key)
}

// Entries ...
func (trs *TestRuntimeStorage) Entries() map[string][]byte {
	iter := trs.db.NewIterator()

	entries := make(map[string][]byte)
	for iter.Next() {
		entries[string(iter.Key())] = iter.Value()
	}

	iter.Release()
	return entries
}

// SetBalance ...
func (trs *TestRuntimeStorage) SetBalance(key [32]byte, balance uint64) error {
	skey, err := common.BalanceKey(key)
	if err != nil {
		return err
	}

	bb := make([]byte, 8)
	binary.LittleEndian.PutUint64(bb, balance)

	return trs.Set(skey, bb)
}

// GetBalance ...
func (trs *TestRuntimeStorage) GetBalance(key [32]byte) (uint64, error) {
	skey, err := common.BalanceKey(key)
	if err != nil {
		return 0, err
	}

	bal, err := trs.Get(skey)
	if err != nil {
		return 0, err
	}

	return binary.LittleEndian.Uint64(bal), nil
}

// DeleteChildStorage ...
func (trs *TestRuntimeStorage) DeleteChildStorage(key []byte) error {
	return trs.trie.DeleteFromChild(key)
}

// ClearChildStorage ...
func (trs *TestRuntimeStorage) ClearChildStorage(keyToChild, key []byte) error {
	return trs.trie.ClearFromChild(keyToChild, key)
}

// TestRuntimeNetwork ...
type TestRuntimeNetwork struct {
}

// NetworkState ...
func (trn *TestRuntimeNetwork) NetworkState() common.NetworkState {
	testAddrs := []ma.Multiaddr(nil)

	// create mock multiaddress
	addr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/7001/p2p/12D3KooWDcCNBqAemRvguPa7rtmsbn2hpgLqAz8KsMMFsF2rdCUP")

	testAddrs = append(testAddrs, addr)

	return common.NetworkState{
		PeerID:     "12D3KooWDcCNBqAemRvguPa7rtmsbn2hpgLqAz8KsMMFsF2rdCUP",
		Multiaddrs: testAddrs,
	}
}
