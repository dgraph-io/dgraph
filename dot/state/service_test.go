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

package state

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/genesis"
	"github.com/ChainSafe/gossamer/lib/trie"
	"github.com/ChainSafe/gossamer/lib/utils"

	log "github.com/ChainSafe/log15"
	"github.com/stretchr/testify/require"
)

var firstEpochInfo = &types.EpochInfo{
	Duration:   200,
	FirstBlock: 0,
}

// helper method to create and start test state service
func newTestService(t *testing.T) (state *Service) {
	testDir := utils.NewTestDir(t)
	state = NewService(testDir, log.LvlTrace)
	return state
}

func newTestMemDBService() *Service {
	state := NewService("", log.LvlTrace)
	state.UseMemDB()
	return state
}

func TestService_Start(t *testing.T) {
	state := newTestService(t)
	defer utils.RemoveTestDir(t)

	genesisHeader, err := types.NewHeader(common.NewHash([]byte{0}), big.NewInt(0), trie.EmptyHash, trie.EmptyHash, [][]byte{})
	require.Nil(t, err)

	tr := trie.NewEmptyTrie()

	genesisData := new(genesis.Data)

	err = state.Initialize(genesisData, genesisHeader, tr, firstEpochInfo)
	require.Nil(t, err)

	err = state.Start()
	require.Nil(t, err)

	err = state.Stop()
	require.Nil(t, err)
}

func TestMemDB_Start(t *testing.T) {
	state := newTestMemDBService()

	genesisHeader, err := types.NewHeader(common.NewHash([]byte{0}), big.NewInt(0), trie.EmptyHash, trie.EmptyHash, [][]byte{})
	require.Nil(t, err)

	tr := trie.NewEmptyTrie()

	genesisData := new(genesis.Data)

	err = state.Initialize(genesisData, genesisHeader, tr, firstEpochInfo)
	require.Nil(t, err)

	err = state.Start()
	require.Nil(t, err)

	err = state.Stop()
	require.Nil(t, err)
}

func TestService_BlockTree(t *testing.T) {
	testDir := utils.NewTestDir(t)

	// removes all data directories created within test directory
	defer utils.RemoveTestDir(t)

	stateA := NewService(testDir, log.LvlTrace)

	genesisHeader, err := types.NewHeader(common.NewHash([]byte{0}), big.NewInt(0), trie.EmptyHash, trie.EmptyHash, [][]byte{})
	require.Nil(t, err)

	genesisData := new(genesis.Data)

	tr := trie.NewEmptyTrie()
	err = stateA.Initialize(genesisData, genesisHeader, tr, firstEpochInfo)
	require.Nil(t, err)

	err = stateA.Start()
	require.Nil(t, err)

	// add blocks to state
	AddBlocksToState(t, stateA.Block, 10)

	err = stateA.Stop()
	require.Nil(t, err)

	stateB := NewService(testDir, log.LvlTrace)

	err = stateB.Start()
	require.Nil(t, err)

	err = stateB.Stop()
	require.Nil(t, err)

	if !reflect.DeepEqual(stateA.Block.BestBlockHash(), stateB.Block.BestBlockHash()) {
		t.Fatalf("Fail: got %s expected %s", stateA.Block.BestBlockHash(), stateB.Block.BestBlockHash())
	}
}
