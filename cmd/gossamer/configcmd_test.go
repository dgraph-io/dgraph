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
	"strings"
	"testing"

	cfg "github.com/ChainSafe/gossamer/config"
	"github.com/ChainSafe/gossamer/config/genesis"
	"github.com/ChainSafe/gossamer/internal/api"
	"github.com/ChainSafe/gossamer/runtime"
	"github.com/ChainSafe/gossamer/state"
	"github.com/ChainSafe/gossamer/tests"
	log "github.com/ChainSafe/log15"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

const TestDataDir = "./test_data"

const TestProtocolID = "/gossamer/test/0"

var TestBootnodes = []string{
	"/dns4/p2p.cc3-0.kusama.network/tcp/30100/p2p/QmeCit3Nif4VfNqrEJsdYHZGcKzRCnZvGxg6hha1iNj4mk",
	"/dns4/p2p.cc3-1.kusama.network/tcp/30100/p2p/QmchDJtEGiEWf7Ag58HNoTg9jSGzxkSZ23VgmF6xiLKKsZ",
}

var TestGenesis = &genesis.Genesis{
	Name:       "gossamer",
	ID:         "gossamer",
	Bootnodes:  TestBootnodes,
	ProtocolID: TestProtocolID,
	Genesis:    genesis.GenesisFields{},
}

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
	testConfig.Global.Authority = false
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

func createTempGenesisFile(t *testing.T) string {
	_ = runtime.NewTestRuntime(t, tests.POLKADOT_RUNTIME)

	testRuntimeFilePath := tests.GetAbsolutePath(tests.POLKADOT_RUNTIME_FP)

	fp, err := filepath.Abs(testRuntimeFilePath)
	require.Nil(t, err)

	testBytes, err := ioutil.ReadFile(fp)
	require.Nil(t, err)

	testHex := hex.EncodeToString(testBytes)
	TestGenesis.Genesis.Raw = [2]map[string]string{}
	if TestGenesis.Genesis.Raw[0] == nil {
		TestGenesis.Genesis.Raw[0] = make(map[string]string)
	}
	TestGenesis.Genesis.Raw[0]["0x3a636f6465"] = "0x" + testHex

	// Create temp file
	file, err := ioutil.TempFile(os.TempDir(), "genesis-test")
	require.Nil(t, err)

	// Grab json encoded bytes
	bz, err := json.Marshal(TestGenesis)
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
		{"roles flag",
			[]string{"datadir", "roles"},
			[]interface{}{"test1", "1"},
			cfg.GlobalConfig{DataDir: tempPath, Roles: byte(1)},
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

func TestCreateNetworkService(t *testing.T) {
	stateSrv := state.NewService(TestDataDir)
	srv, _, _ := createNetworkService(cfg.DefaultConfig(), &genesis.GenesisData{}, stateSrv)
	require.NotNil(t, srv, "failed to create network service")
}

func TestSetNetworkConfig(t *testing.T) {
	tempFile, cfgClone := createTempConfigFile()
	app := cli.NewApp()
	app.Writer = ioutil.Discard
	tc := []struct {
		description string
		flags       []string
		values      []interface{}
		expected    cfg.NetworkCfg
	}{
		{
			"config file",
			[]string{"config"},
			[]interface{}{tempFile.Name()},
			cfgClone.Network,
		},
		{
			"no bootstrap, no mdns",
			[]string{"nobootstrap", "nomdns"},
			[]interface{}{true, true},
			cfg.NetworkCfg{
				Bootnodes:   cfg.DefaultNetworkBootnodes,
				ProtocolID:  cfg.DefaultNetworkProtocolID,
				Port:        cfg.DefaultNetworkPort,
				NoBootstrap: true,
				NoMdns:      true,
			},
		},
		{
			"bootstrap nodes",
			[]string{"bootnodes"},
			[]interface{}{strings.Join(TestBootnodes[:], ",")},
			cfg.NetworkCfg{
				Bootnodes:   TestBootnodes,
				ProtocolID:  cfg.DefaultNetworkProtocolID,
				Port:        cfg.DefaultNetworkPort,
				NoBootstrap: false,
				NoMdns:      false,
			},
		},
		{
			"port",
			[]string{"port"},
			[]interface{}{uint(1337)},
			cfg.NetworkCfg{
				Bootnodes:   cfg.DefaultNetworkBootnodes,
				ProtocolID:  cfg.DefaultNetworkProtocolID,
				Port:        1337,
				NoBootstrap: false,
				NoMdns:      false,
			},
		},
		{
			"protocol id",
			[]string{"protocol"},
			[]interface{}{TestProtocolID},
			cfg.NetworkCfg{
				Bootnodes:   cfg.DefaultNetworkBootnodes,
				Port:        cfg.DefaultNetworkPort,
				ProtocolID:  TestProtocolID,
				NoBootstrap: false,
				NoMdns:      false,
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
			setNetworkConfig(context, &input.Network)

			require.Equal(t, c.expected, input.Network)
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
	tempFile, cfgClone := createTempConfigFile()
	defer teardown(tempFile)
	defer removeTestDataDir()

	genesisPath := createTempGenesisFile(t)
	defer os.Remove(genesisPath)

	app := cli.NewApp()
	app.Writer = ioutil.Discard
	tc := []struct {
		name     string
		flags    []string
		values   []interface{}
		expected *cfg.Config
	}{
		{"node from config (norpc)", []string{"config", "genesis"}, []interface{}{tempFile.Name(), genesisPath}, cfgClone},
		{"default node (norpc)", []string{"genesis"}, []interface{}{genesisPath}, cfgClone},
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
