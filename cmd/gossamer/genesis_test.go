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

package main

import (
	"bytes"
	"flag"
	"math/big"
	"os"
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/dot/core"
	"github.com/ChainSafe/gossamer/dot/core/types"
	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/genesis"
	"github.com/ChainSafe/gossamer/lib/trie"
	"github.com/ChainSafe/gossamer/node"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

func TestStoreGenesisInfo(t *testing.T) {
	tempFile, cfg := createTempConfigFile()
	defer teardown(tempFile)

	genesisPath := createTempGenesisFile(t)
	defer os.Remove(genesisPath)

	set := flag.NewFlagSet("config", 0)
	set.String("config", tempFile.Name(), "TOML configuration file")
	set.String("genesis", genesisPath, "path to genesis file")
	ctx := cli.NewContext(nil, set, nil)

	err := loadGenesis(ctx)
	require.Nil(t, err)

	currentConfig, err := getConfig(ctx)
	require.Nil(t, err)

	dataDir := cfg.Global.DataDir

	dbSrv := state.NewService(dataDir)
	err = dbSrv.Initialize(&types.Header{
		Number:         big.NewInt(0),
		StateRoot:      trie.EmptyHash,
		ExtrinsicsRoot: trie.EmptyHash,
	}, trie.NewEmptyTrie(nil))
	require.Nil(t, err)

	err = dbSrv.Start()
	require.Nil(t, err)

	defer dbSrv.Stop()

	setGlobalConfig(ctx, &currentConfig.Global)

	gendata, err := dbSrv.Storage.LoadGenesisData()
	require.Nil(t, err)

	expected := &genesis.Data{
		Name:       TestGenesis.Name,
		ID:         TestGenesis.ID,
		Bootnodes:  common.StringArrayToBytes(TestGenesis.Bootnodes),
		ProtocolID: TestGenesis.ProtocolID,
	}

	if !reflect.DeepEqual(gendata, expected) {
		t.Fatalf("Fail to get genesis data: got %s expected %s", gendata, expected)
	}

	stateRoot := dbSrv.Block.LatestHeader().StateRoot
	expectedHeader, err := types.NewHeader(common.NewHash([]byte{0}), big.NewInt(0), stateRoot, trie.EmptyHash, [][]byte{})
	if err != nil {
		t.Fatal(err)
	}

	genesisHeader := dbSrv.Block.LatestHeader()
	if !genesisHeader.Hash().Equal(expectedHeader.Hash()) {
		t.Fatalf("Fail: got %v expected %v", genesisHeader, expectedHeader)
	}
}

func TestGenesisStateLoading(t *testing.T) {
	tempFile, _ := createTempConfigFile()
	defer teardown(tempFile)

	genesisPath := createTempGenesisFile(t)
	defer os.Remove(genesisPath)

	gen, err := genesis.LoadGenesisFromJSON(genesisPath)
	if err != nil {
		t.Fatal(err)
	}

	set := flag.NewFlagSet("config", 0)
	set.String("config", tempFile.Name(), "TOML configuration file")
	set.String("genesis", genesisPath, "path to genesis file")
	set.Bool("authority", false, "")
	context := cli.NewContext(nil, set, nil)

	err = loadGenesis(context)
	require.Nil(t, err)

	d, _, err := makeNode(context)
	require.Nil(t, err)

	if reflect.TypeOf(d) != reflect.TypeOf(&node.Node{}) {
		t.Fatalf("failed to return correct type: got %v expected %v", reflect.TypeOf(d), reflect.TypeOf(&node.Node{}))
	}

	expected := &trie.Trie{}
	err = expected.Load(gen.GenesisFields().Raw[0])
	require.Nil(t, err)

	expectedRoot, err := expected.Hash()
	require.Nil(t, err)

	mgr := d.Services.Get(&core.Service{})

	var coreSrv *core.Service
	var ok bool

	if coreSrv, ok = mgr.(*core.Service); !ok {
		t.Fatal("could not find core service")
	}

	if coreSrv == nil {
		t.Fatal("core service is nil")
	}

	stateRoot, err := coreSrv.StorageRoot()
	require.Nil(t, err)

	if !bytes.Equal(expectedRoot[:], stateRoot[:]) {
		t.Fatalf("Fail: got %x expected %x", stateRoot, expectedRoot)
	}
}
