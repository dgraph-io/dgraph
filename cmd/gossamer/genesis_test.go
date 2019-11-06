package main

import (
	"bytes"
	"flag"
	"os"
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/config/genesis"
	"github.com/ChainSafe/gossamer/core"
	"github.com/ChainSafe/gossamer/dot"
	"github.com/ChainSafe/gossamer/polkadb"
	"github.com/ChainSafe/gossamer/trie"
	"github.com/urfave/cli"
)

func TestStoreGenesisInfo(t *testing.T) {
	tempFile, _ := createTempConfigFile()
	defer teardown(tempFile)

	genesispath := createTempGenesisFile(t)
	defer os.Remove(genesispath)

	set := flag.NewFlagSet("config", 0)
	set.String("config", tempFile.Name(), "TOML configuration file")
	set.String("genesis", genesispath, "genesis file")
	ctx := cli.NewContext(nil, set, nil)

	err := loadGenesis(ctx)
	if err != nil {
		t.Fatal(err)
	}

	fig, err := getConfig(ctx)
	if err != nil {
		t.Fatal(err)
	}

	setGlobalConfig(ctx, &fig.Global)
	dbSrv, err := polkadb.NewDbService(fig.Global.DataDir)
	if err != nil {
		t.Fatal(err)
	}

	err = dbSrv.Start()
	if err != nil {
		t.Fatal(err)
	}

	defer dbSrv.Stop()

	tdb := &trie.Database{
		Db: dbSrv.StateDB.Db,
	}

	gendata, err := tdb.LoadGenesisData()
	if err != nil {
		t.Fatal(err)
	}

	expected := &genesis.GenesisData{
		Name:       tmpGenesis.Name,
		Id:         tmpGenesis.Id,
		ProtocolId: tmpGenesis.ProtocolId,
		Bootnodes:  common.StringArrayToBytes(tmpGenesis.Bootnodes),
	}

	if !reflect.DeepEqual(gendata, expected) {
		t.Fatalf("Fail to get genesis data: got %s expected %s", gendata, expected)
	}
}

func TestGenesisStateLoading(t *testing.T) {
	tempFile, _ := createTempConfigFile()
	defer teardown(tempFile)

	genesispath := createTempGenesisFile(t)
	defer os.Remove(genesispath)

	gen, err := genesis.LoadGenesisJsonFile(genesispath)
	if err != nil {
		t.Fatal(err)
	}

	set := flag.NewFlagSet("config", 0)
	set.String("config", tempFile.Name(), "TOML configuration file")
	set.String("genesis", genesispath, "genesis file")
	context := cli.NewContext(nil, set, nil)

	err = loadGenesis(context)
	if err != nil {
		t.Fatal(err)
	}

	d, _, err := makeNode(context)
	if err != nil {
		t.Fatal(err)
	}

	if reflect.TypeOf(d) != reflect.TypeOf(&dot.Dot{}) {
		t.Fatalf("failed to return correct type: got %v expected %v", reflect.TypeOf(d), reflect.TypeOf(&dot.Dot{}))
	}

	expected := &trie.Trie{}
	err = expected.Load(gen.Genesis.Raw)
	if err != nil {
		t.Fatal(err)
	}

	expectedRoot, err := expected.Hash()
	if err != nil {
		t.Fatal(err)
	}

	mgr := d.Services.Get(&core.Service{})

	stateRoot, err := mgr.(*core.Service).StorageRoot()
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(expectedRoot[:], stateRoot[:]) {
		t.Fatalf("Fail: got %x expected %x", stateRoot, expectedRoot)
	}
}
