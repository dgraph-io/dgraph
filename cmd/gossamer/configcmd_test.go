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
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ChainSafe/gossamer/runtime"
	"github.com/ChainSafe/gossamer/tests"

	"github.com/ChainSafe/gossamer/state"

	cfg "github.com/ChainSafe/gossamer/config"
	"github.com/ChainSafe/gossamer/config/genesis"
	"github.com/ChainSafe/gossamer/internal/api"
	log "github.com/ChainSafe/log15"
	"github.com/urfave/cli"
)

const TestDataDir = "./test_data"

func teardown(tempFile *os.File) {
	if err := os.Remove(tempFile.Name()); err != nil {
		log.Warn("cannot remove temp file", "err", err)
	}
}

func removeTestDataDir() {
	if err := os.RemoveAll(TestDataDir); err != nil {
		log.Warn("cannot remove test data dir", "err", err)
	}
}

func createTempConfigFile() (*os.File, *cfg.Config) {
	testConfig := cfg.DefaultConfig()
	testConfig.Global.DataDir = TestDataDir

	tmpFile, err := ioutil.TempFile(os.TempDir(), "prefix-")
	if err != nil {
		log.Crit("Cannot create temporary file", "err", err)
		os.Exit(1)
	}

	f := cfg.ToTOML(tmpFile.Name(), testConfig)
	return f, testConfig
}

// Creates a cli context for a test given a set of flags and values
func createCliContext(description string, flags []string, values []interface{}) (*cli.Context, error) {
	set := flag.NewFlagSet(description, 0)
	for i := range values {
		switch v := values[i].(type) {
		case bool:
			set.Bool(flags[i], v, "")
		case string:
			set.String(flags[i], v, "")
		case uint:
			set.Uint(flags[i], v, "")
		default:
			return nil, fmt.Errorf("unexpected cli value type: %T", values[i])
		}
	}
	context := cli.NewContext(nil, set, nil)
	return context, nil
}

var tmpGenesis = &genesis.Genesis{
	Name:       "gossamer",
	ID:         "gossamer",
	Bootnodes:  []string{"/ip4/104.211.54.233/tcp/30363/p2p/16Uiu2HAmFWPUx45xYYeCpAryQbvU3dY8PWGdMwS2tLm1dB1CsmCj"},
	ProtocolID: "gossamer",
	Genesis:    genesis.GenesisFields{},
}

func createTempGenesisFile(t *testing.T) string {
	_ = runtime.NewTestRuntime(t, tests.POLKADOT_RUNTIME)

	testRuntimeFilePath := tests.GetAbsolutePath(tests.POLKADOT_RUNTIME_FP)

	fp, err := filepath.Abs(testRuntimeFilePath)
	require.Nil(t, err)

	testBytes, err := ioutil.ReadFile(fp)
	require.Nil(t, err)

	testHex := hex.EncodeToString(testBytes)
	testRaw := [2]map[string]string{}
	testRaw[0] = map[string]string{"0x3a636f6465": "0x" + testHex}
	tmpGenesis.Genesis = genesis.GenesisFields{Raw: testRaw}

	// Create temp file
	file, err := ioutil.TempFile("", "genesis-test")
	require.Nil(t, err)

	// Grab json encoded bytes
	bz, err := json.Marshal(tmpGenesis)
	require.Nil(t, err)

	// Write to temp file
	_, err = file.Write(bz)
	require.Nil(t, err)

	return file.Name()
}

func TestGetConfig(t *testing.T) {
	tempFile, cfgClone := createTempConfigFile()
	defer teardown(tempFile)

	var err error
	cfgClone.Global.DataDir, err = filepath.Abs(cfgClone.Global.DataDir)
	require.Nil(t, err)

	app := cli.NewApp()
	app.Writer = ioutil.Discard

	tc := []struct {
		name     string
		value    string
		expected *cfg.Config
	}{
		{"", "", cfg.DefaultConfig()},
		{"config", tempFile.Name(), cfgClone},
	}

	for _, c := range tc {
		set := flag.NewFlagSet(c.name, 0)
		set.String(c.name, c.value, "")
		context := cli.NewContext(app, set, nil)

		currentConfig, err := getConfig(context)
		require.Nil(t, err)

		require.Equal(t, c.expected, currentConfig)
	}
}

func TestSetGlobalConfig(t *testing.T) {
	tempPath, _ := filepath.Abs("test1")
	app := cli.NewApp()
	app.Writer = ioutil.Discard
	tc := []struct {
		description string
		flags       []string
		values      []interface{}
		expected    cfg.GlobalConfig
	}{
		{"datadir flag",
			[]string{"datadir"},
			[]interface{}{"test1"},
			cfg.GlobalConfig{DataDir: tempPath},
		},
	}

	for _, c := range tc {
		c := c // bypass scopelint false positive
		t.Run(c.description, func(t *testing.T) {
			context, err := createCliContext(c.description, c.flags, c.values)
			require.Nil(t, err)

			tCfg := &cfg.GlobalConfig{}

			setGlobalConfig(context, tCfg)

			require.Equal(t, c.expected, *tCfg)
		})
	}
}

func TestCreateP2PService(t *testing.T) {
	gendata := &genesis.GenesisData{
		ProtocolID: "/gossamer/test",
	}

	srv, _, _ := createP2PService(cfg.DefaultConfig(), gendata)
	require.NotNil(t, srv, "failed to create p2p service")
}

func TestSetP2pConfig(t *testing.T) {
	tempFile, cfgClone := createTempConfigFile()
	app := cli.NewApp()
	app.Writer = ioutil.Discard
	tc := []struct {
		description string
		flags       []string
		values      []interface{}
		expected    cfg.P2pCfg
	}{
		{
			"config file",
			[]string{"config"},
			[]interface{}{tempFile.Name()},
			cfgClone.P2p,
		},
		{
			"no bootstrap, no mdns",
			[]string{"nobootstrap", "nomdns"},
			[]interface{}{true, true},
			cfg.P2pCfg{
				BootstrapNodes: cfg.DefaultP2PBootstrap,
				Port:           cfg.DefaultP2PPort,
				ProtocolID:     cfg.DefaultP2PProtocolID,
				NoBootstrap:    true,
				NoMdns:         true,
			},
		},
		{
			"bootstrap nodes",
			[]string{"bootnodes"},
			[]interface{}{"1234,5678"},
			cfg.P2pCfg{
				BootstrapNodes: []string{"1234", "5678"},
				Port:           cfg.DefaultP2PPort,
				ProtocolID:     cfg.DefaultP2PProtocolID,
				NoBootstrap:    false,
				NoMdns:         false,
			},
		},
		{
			"port",
			[]string{"p2pport"},
			[]interface{}{uint(1337)},
			cfg.P2pCfg{
				BootstrapNodes: cfg.DefaultP2PBootstrap,
				Port:           1337,
				ProtocolID:     cfg.DefaultP2PProtocolID,
				NoBootstrap:    false,
				NoMdns:         false,
			},
		},
		{
			"protocol id",
			[]string{"protocol"},
			[]interface{}{"/gossamer/test"},
			cfg.P2pCfg{
				BootstrapNodes: cfg.DefaultP2PBootstrap,
				Port:           cfg.DefaultP2PPort,
				ProtocolID:     "/gossamer/test",
				NoBootstrap:    false,
				NoMdns:         false,
			},
		},
	}

	for _, c := range tc {
		c := c // bypass scopelint false positive
		t.Run(c.description, func(t *testing.T) {
			context, err := createCliContext(c.description, c.flags, c.values)
			require.Nil(t, err)

			input := cfg.DefaultConfig()
			// Must call global setup to set data dir
			setP2pConfig(context, &input.P2p)

			require.Equal(t, c.expected, input.P2p)
		})
	}
}

func TestSetRPCConfig(t *testing.T) {
	tempFile, cfgClone := createTempConfigFile()

	app := cli.NewApp()
	app.Writer = ioutil.Discard
	tc := []struct {
		description string
		flags       []string
		values      []interface{}
		expected    cfg.RPCCfg
	}{
		{
			"config file",
			[]string{"config"},
			[]interface{}{tempFile.Name()},
			cfgClone.RPC,
		},
		{
			"host and port",
			[]string{"rpchost", "rpcport"},
			[]interface{}{"someHost", uint(1337)},
			cfg.RPCCfg{
				Port:    1337,
				Host:    "someHost",
				Modules: cfg.DefaultRPCModules,
			},
		},
		{
			"modules",
			[]string{"rpcmods"},
			[]interface{}{"system,state"},
			cfg.RPCCfg{
				Port:    cfg.DefaultRPCHTTPPort,
				Host:    cfg.DefaultRPCHTTPHost,
				Modules: []api.Module{"system", "state"},
			},
		},
	}

	for _, c := range tc {
		c := c // bypass scopelint false positive
		t.Run(c.description, func(t *testing.T) {
			context, err := createCliContext(c.description, c.flags, c.values)
			require.Nil(t, err)

			input := cfg.DefaultConfig()
			setRPCConfig(context, &input.RPC)

			require.Equal(t, c.expected, input.RPC)
		})
	}
}

func TestStrToMods(t *testing.T) {
	strs := []string{"test1", "test2"}
	mods := strToMods(strs)
	rv := reflect.ValueOf(mods)
	if rv.Kind() == reflect.Ptr {
		t.Fatalf("test failed: got %v expected %v", mods, &[]api.Module{})
	}
}

func TestMakeNode(t *testing.T) {
	t.Skip()
	tempFile, cfgClone := createTempConfigFile()
	defer teardown(tempFile)
	defer removeTestDataDir()

	genesispath := createTempGenesisFile(t)
	defer os.Remove(genesispath)

	app := cli.NewApp()
	app.Writer = ioutil.Discard
	tc := []struct {
		name     string
		flags    []string
		values   []interface{}
		expected *cfg.Config
	}{
		{"node from config (norpc)", []string{"config", "genesis"}, []interface{}{tempFile.Name(), genesispath}, cfgClone},
		{"default node (norpc)", []string{"genesis"}, []interface{}{genesispath}, cfgClone},
	}

	for _, c := range tc {
		c := c // bypass scopelint false positive

		t.Run(c.name, func(t *testing.T) {
			context, err := createCliContext(c.name, c.flags, c.values)
			require.Nil(t, err)

			err = loadGenesis(context)
			require.Nil(t, err)

			node, _, err := makeNode(context)
			require.Nil(t, err)

			db := node.Services.Get(&state.Service{})

			err = db.Stop()
			require.Nil(t, err)
		})
	}
}

func TestCommands(t *testing.T) {
	tempFile, _ := createTempConfigFile()
	defer teardown(tempFile)
	defer removeTestDataDir()

	tc := []struct {
		description string
		flags       []string
		values      []interface{}
	}{
		{"from config file",
			[]string{"config"},
			[]interface{}{tempFile.Name()}},
	}

	for _, c := range tc {
		c := c // bypass scopelint false positive

		app := cli.NewApp()
		app.Writer = ioutil.Discard

		context, err := createCliContext(c.description, c.flags, c.values)
		require.Nil(t, err)

		command := dumpConfigCommand

		err = command.Run(context)
		require.Nil(t, err, "should have ran dumpConfig command.")
	}
}
