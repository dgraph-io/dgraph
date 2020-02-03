package main

import (
	"bytes"
	"flag"
	"math/big"
	"os"
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/config/genesis"
	"github.com/ChainSafe/gossamer/core"
	"github.com/ChainSafe/gossamer/core/types"
	"github.com/ChainSafe/gossamer/dot"
	"github.com/ChainSafe/gossamer/state"
	"github.com/ChainSafe/gossamer/trie"
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

	expected := &genesis.GenesisData{
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
	context := cli.NewContext(nil, set, nil)

	err = loadGenesis(context)
	require.Nil(t, err)

	d, _, err := makeNode(context)
	require.Nil(t, err)

	if reflect.TypeOf(d) != reflect.TypeOf(&dot.Dot{}) {
		t.Fatalf("failed to return correct type: got %v expected %v", reflect.TypeOf(d), reflect.TypeOf(&dot.Dot{}))
	}

	expected := &trie.Trie{}
	err = expected.Load(gen.GenesisFields().Raw[0])
	require.Nil(t, err)

	expectedRoot, err := expected.Hash()
	require.Nil(t, err)

	mgr := d.Services.Get(&core.Service{})

	stateRoot, err := mgr.(*core.Service).StorageRoot()
	require.Nil(t, err)

	if !bytes.Equal(expectedRoot[:], stateRoot[:]) {
		t.Fatalf("Fail: got %x expected %x", stateRoot, expectedRoot)
	}
}
