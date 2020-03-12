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

package dot

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"math/big"
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/dot/core"
	"github.com/ChainSafe/gossamer/dot/core/types"
	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/genesis"
	"github.com/ChainSafe/gossamer/lib/keystore"
	"github.com/ChainSafe/gossamer/lib/runtime"
	"github.com/ChainSafe/gossamer/lib/trie"
	"github.com/ChainSafe/gossamer/lib/utils"
	"github.com/ChainSafe/gossamer/tests"

	"github.com/stretchr/testify/require"
)

// newTestGenesisAndRuntime create a new test runtime and a new test genesis
// file with the test runtime stored in raw data and returns the genesis file
func newTestGenesisAndRuntime(t *testing.T) string {
	dir := utils.NewTestDir(t)

	_ = runtime.NewTestRuntime(t, tests.POLKADOT_RUNTIME)
	runtimeFilePath := tests.GetAbsolutePath(tests.POLKADOT_RUNTIME_FP)

	runtimeData, err := ioutil.ReadFile(runtimeFilePath)
	require.Nil(t, err)

	gen := NewTestGenesis(t)
	hex := hex.EncodeToString(runtimeData)

	gen.Genesis.Raw = [2]map[string]string{}
	if gen.Genesis.Raw[0] == nil {
		gen.Genesis.Raw[0] = make(map[string]string)
	}
	gen.Genesis.Raw[0]["0x3a636f6465"] = "0x" + hex

	genFile, err := ioutil.TempFile(dir, "genesis-")
	require.Nil(t, err)

	genData, err := json.Marshal(gen)
	require.Nil(t, err)

	_, err = genFile.Write(genData)
	require.Nil(t, err)

	return genFile.Name()
}

// TestStoreGenesisInfo
func TestStoreGenesisInfo(t *testing.T) {
	cfg := NewTestConfig(t)
	require.NotNil(t, cfg)

	genPath := newTestGenesisAndRuntime(t)
	require.NotNil(t, genPath)

	defer utils.RemoveTestDir(t)

	cfg.Global.Genesis = genPath

	err := InitNode(cfg)
	require.Nil(t, err)

	dataDir := cfg.Global.DataDir

	stateSrvc := state.NewService(dataDir)
	err = stateSrvc.Initialize(&types.Header{
		Number:         big.NewInt(0),
		StateRoot:      trie.EmptyHash,
		ExtrinsicsRoot: trie.EmptyHash,
	}, trie.NewEmptyTrie(nil))
	require.Nil(t, err)

	err = stateSrvc.Start()
	require.Nil(t, err)

	defer stateSrvc.Stop()

	gendata, err := stateSrvc.Storage.LoadGenesisData()
	require.Nil(t, err)

	testGenesis := NewTestGenesis(t)

	expected := &genesis.Data{
		Name:       testGenesis.Name,
		ID:         testGenesis.ID,
		Bootnodes:  common.StringArrayToBytes(testGenesis.Bootnodes),
		ProtocolID: testGenesis.ProtocolID,
	}

	if !reflect.DeepEqual(gendata, expected) {
		t.Fatalf("Fail to get genesis data: got %s expected %s", gendata, expected)
	}

	genesisHeader, err := stateSrvc.Block.BestBlockHeader()
	if err != nil {
		t.Fatal(err)
	}

	stateRoot := genesisHeader.StateRoot
	expectedHeader, err := types.NewHeader(common.NewHash([]byte{0}), big.NewInt(0), stateRoot, trie.EmptyHash, [][]byte{})
	if err != nil {
		t.Fatal(err)
	}

	if !genesisHeader.Hash().Equal(expectedHeader.Hash()) {
		t.Fatalf("Fail: got %v expected %v", genesisHeader, expectedHeader)
	}
}

// TestGenesisStateLoading
func TestGenesisStateLoading(t *testing.T) {
	cfg := NewTestConfig(t)
	require.NotNil(t, cfg)

	genPath := newTestGenesisAndRuntime(t)
	require.NotNil(t, genPath)

	defer utils.RemoveTestDir(t)

	cfg.Core.Authority = false // TODO: improve dot genesis tests
	cfg.Global.Genesis = genPath

	gen, err := genesis.LoadGenesisFromJSON(genPath)
	if err != nil {
		t.Fatal(err)
	}

	err = InitNode(cfg)
	require.Nil(t, err)

	ks := keystore.NewKeystore()
	require.NotNil(t, ks)

	node, err := NewNode(cfg, ks)
	require.Nil(t, err)

	if reflect.TypeOf(node) != reflect.TypeOf(&Node{}) {
		t.Fatalf("failed to return correct type: got %v expected %v", reflect.TypeOf(node), reflect.TypeOf(&Node{}))
	}

	expected := &trie.Trie{}
	err = expected.Load(gen.GenesisFields().Raw[0])
	require.Nil(t, err)

	expectedRoot, err := expected.Hash()
	require.Nil(t, err)

	mgr := node.Services.Get(&core.Service{})

	var coreSrvc *core.Service
	var ok bool

	if coreSrvc, ok = mgr.(*core.Service); !ok {
		t.Fatal("could not find core service")
	}

	if coreSrvc == nil {
		t.Fatal("core service is nil")
	}

	stateRoot, err := coreSrvc.StorageRoot()
	require.Nil(t, err)

	if !bytes.Equal(expectedRoot[:], stateRoot[:]) {
		t.Fatalf("Fail: got %x expected %x", stateRoot, expectedRoot)
	}
}
