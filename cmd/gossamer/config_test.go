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
	"io/ioutil"
	"testing"

	"github.com/ChainSafe/gossamer/chain/gssmr"
	"github.com/ChainSafe/gossamer/dot"
	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/lib/genesis"
	"github.com/ChainSafe/gossamer/lib/utils"

	database "github.com/ChainSafe/chaindb"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

// TODO: TestSetDotGlobalConfig - add cmd config tests

// TODO: TestSetDotAccountConfig - add cmd config tests

// TODO: TestSetDotCoreConfig - add cmd config tests

// TODO: TestSetDotNetworkConfig - add cmd config tests

// TODO: TestSetDotRPCConfig - add cmd config tests

// TestConfigFromChainFlag tests createDotConfig using the --chain flag
func TestConfigFromChainFlag(t *testing.T) {
	testApp := cli.NewApp()
	testApp.Writer = ioutil.Discard

	gssmrCfg := dot.GssmrConfig()
	gssmrCfg.Log = dot.LogConfig{
		CoreLvl:           gssmr.DefaultLvl,
		SyncLvl:           gssmr.DefaultLvl,
		NetworkLvl:        gssmr.DefaultLvl,
		RPCLvl:            gssmr.DefaultLvl,
		StateLvl:          gssmr.DefaultLvl,
		RuntimeLvl:        gssmr.DefaultLvl,
		BlockProducerLvl:  gssmr.DefaultLvl,
		FinalityGadgetLvl: gssmr.DefaultLvl,
	}

	testcases := []struct {
		description string
		flags       []string
		values      []interface{}
		expected    *dot.Config
	}{
		{
			"Test gossamer --chain gssmr",
			[]string{"chain"},
			[]interface{}{"gssmr"},
			gssmrCfg,
		},
		{
			"Test gossamer --chain ksmcc",
			[]string{"chain"},
			[]interface{}{"ksmcc"},
			dot.KsmccConfig(),
		},
	}

	for _, c := range testcases {
		c := c // bypass scopelint false positive
		t.Run(c.description, func(t *testing.T) {
			ctx, err := newTestContext(c.description, c.flags, c.values)
			require.Nil(t, err)
			cfg, err := createDotConfig(ctx)
			require.Nil(t, err)
			require.Equal(t, c.expected, cfg)
		})
	}
}

// TestInitConfigFromFlags tests createDotInitConfig using relevant init flags
func TestInitConfigFromFlags(t *testing.T) {
	testCfg, testCfgFile := dot.NewTestConfigWithFile(t)
	require.NotNil(t, testCfg)
	require.NotNil(t, testCfgFile)

	defer utils.RemoveTestDir(t)

	testApp := cli.NewApp()
	testApp.Writer = ioutil.Discard

	testcases := []struct {
		description string
		flags       []string
		values      []interface{}
		expected    dot.InitConfig
	}{
		{
			"Test gossamer --genesis-raw",
			[]string{"config", "genesis-raw"},
			[]interface{}{testCfgFile.Name(), "test_genesis"},
			dot.InitConfig{
				GenesisRaw:     "test_genesis",
				TestFirstEpoch: true,
			},
		},
	}

	for _, c := range testcases {
		c := c // bypass scopelint false positive
		t.Run(c.description, func(t *testing.T) {
			ctx, err := newTestContext(c.description, c.flags, c.values)
			require.Nil(t, err)
			cfg, err := createInitConfig(ctx)
			require.Nil(t, err)
			require.Equal(t, c.expected, cfg.Init)
		})
	}
}

// TestGlobalConfigFromFlags tests createDotGlobalConfig using relevant global flags
func TestGlobalConfigFromFlags(t *testing.T) {
	testCfg, testCfgFile := dot.NewTestConfigWithFile(t)
	require.NotNil(t, testCfg)
	require.NotNil(t, testCfgFile)

	defer utils.RemoveTestDir(t)

	testApp := cli.NewApp()
	testApp.Writer = ioutil.Discard

	testcases := []struct {
		description string
		flags       []string
		values      []interface{}
		expected    dot.GlobalConfig
	}{
		{
			"Test gossamer --config",
			[]string{"config"},
			[]interface{}{testCfgFile.Name()},
			dot.GlobalConfig{
				Name:     testCfg.Global.Name,
				ID:       testCfg.Global.ID,
				BasePath: testCfg.Global.BasePath,
				LogLevel: "info",
			},
		},
		{
			"Test gossamer --chain",
			[]string{"config", "chain"},
			[]interface{}{testCfgFile.Name(), "ksmcc"},
			dot.GlobalConfig{
				Name:     testCfg.Global.Name,
				ID:       "ksmcc",
				BasePath: testCfg.Global.BasePath,
				LogLevel: "info",
			},
		},
		{
			"Test gossamer --name",
			[]string{"config", "name"},
			[]interface{}{testCfgFile.Name(), "test_name"},
			dot.GlobalConfig{
				Name:     "test_name",
				ID:       testCfg.Global.ID,
				BasePath: testCfg.Global.BasePath,
				LogLevel: "info",
			},
		},
		{
			"Test gossamer --basepath",
			[]string{"config", "basepath"},
			[]interface{}{testCfgFile.Name(), "test_basepath"},
			dot.GlobalConfig{
				Name:     testCfg.Global.Name,
				ID:       testCfg.Global.ID,
				BasePath: "test_basepath",
				LogLevel: "info",
			},
		},
		{
			"Test gossamer --roles",
			[]string{"config", "roles"},
			[]interface{}{testCfgFile.Name(), "1"},
			dot.GlobalConfig{
				Name:     testCfg.Global.Name,
				ID:       testCfg.Global.ID,
				BasePath: testCfg.Global.BasePath,
				LogLevel: "info",
			},
		},
	}

	for _, c := range testcases {
		c := c // bypass scopelint false positive
		t.Run(c.description, func(t *testing.T) {
			ctx, err := newTestContext(c.description, c.flags, c.values)
			require.Nil(t, err)
			cfg, err := createDotConfig(ctx)
			require.Nil(t, err)

			require.Equal(t, c.expected, cfg.Global)
		})
	}
}

// TestAccountConfigFromFlags tests createDotAccountConfig using relevant account flags
func TestAccountConfigFromFlags(t *testing.T) {
	testCfg, testCfgFile := dot.NewTestConfigWithFile(t)
	require.NotNil(t, testCfg)
	require.NotNil(t, testCfgFile)

	defer utils.RemoveTestDir(t)

	testApp := cli.NewApp()
	testApp.Writer = ioutil.Discard

	testcases := []struct {
		description string
		flags       []string
		values      []interface{}
		expected    dot.AccountConfig
	}{
		{
			"Test gossamer --key",
			[]string{"config", "key"},
			[]interface{}{testCfgFile.Name(), "alice"},
			dot.AccountConfig{
				Key:    "alice",
				Unlock: testCfg.Account.Unlock,
			},
		},
		{
			"Test gossamer --unlock",
			[]string{"config", "unlock"},
			[]interface{}{testCfgFile.Name(), "0"},
			dot.AccountConfig{
				Key:    testCfg.Account.Key,
				Unlock: "0",
			},
		},
	}

	for _, c := range testcases {
		c := c // bypass scopelint false positive
		t.Run(c.description, func(t *testing.T) {
			ctx, err := newTestContext(c.description, c.flags, c.values)
			require.Nil(t, err)
			cfg, err := createDotConfig(ctx)
			require.Nil(t, err)
			require.Equal(t, c.expected, cfg.Account)
		})
	}
}

// TestCoreConfigFromFlags tests createDotCoreConfig using relevant core flags
func TestCoreConfigFromFlags(t *testing.T) {
	testCfg, testCfgFile := dot.NewTestConfigWithFile(t)
	require.NotNil(t, testCfg)
	require.NotNil(t, testCfgFile)

	defer utils.RemoveTestDir(t)

	testApp := cli.NewApp()
	testApp.Writer = ioutil.Discard

	testcases := []struct {
		description string
		flags       []string
		values      []interface{}
		expected    dot.CoreConfig
	}{
		{
			"Test gossamer --roles",
			[]string{"config", "roles"},
			[]interface{}{testCfgFile.Name(), "4"},
			dot.CoreConfig{
				Authority:        true,
				Roles:            4,
				BabeAuthority:    true,
				GrandpaAuthority: true,
			},
		},
		{
			"Test gossamer --roles",
			[]string{"config", "roles"},
			[]interface{}{testCfgFile.Name(), "0"},
			dot.CoreConfig{
				Authority:        false,
				Roles:            0,
				BabeAuthority:    false,
				GrandpaAuthority: false,
			},
		},
	}

	for _, c := range testcases {
		c := c // bypass scopelint false positive
		t.Run(c.description, func(t *testing.T) {
			ctx, err := newTestContext(c.description, c.flags, c.values)
			require.Nil(t, err)
			cfg, err := createDotConfig(ctx)
			require.Nil(t, err)
			require.Equal(t, c.expected, cfg.Core)
		})
	}
}

// TestNetworkConfigFromFlags tests createDotNetworkConfig using relevant network flags
func TestNetworkConfigFromFlags(t *testing.T) {
	testCfg, testCfgFile := dot.NewTestConfigWithFile(t)
	require.NotNil(t, testCfg)
	require.NotNil(t, testCfgFile)

	defer utils.RemoveTestDir(t)

	testApp := cli.NewApp()
	testApp.Writer = ioutil.Discard

	testcases := []struct {
		description string
		flags       []string
		values      []interface{}
		expected    dot.NetworkConfig
	}{
		{
			"Test gossamer --port",
			[]string{"config", "port"},
			[]interface{}{testCfgFile.Name(), "1234"},
			dot.NetworkConfig{
				Port:        1234,
				Bootnodes:   testCfg.Network.Bootnodes,
				ProtocolID:  testCfg.Network.ProtocolID,
				NoBootstrap: testCfg.Network.NoBootstrap,
				NoMDNS:      testCfg.Network.NoMDNS,
			},
		},
		{
			"Test gossamer --bootnodes",
			[]string{"config", "bootnodes"},
			[]interface{}{testCfgFile.Name(), "peer1,peer2"},
			dot.NetworkConfig{
				Port:        testCfg.Network.Port,
				Bootnodes:   []string{"peer1", "peer2"},
				ProtocolID:  testCfg.Network.ProtocolID,
				NoBootstrap: testCfg.Network.NoBootstrap,
				NoMDNS:      testCfg.Network.NoMDNS,
			},
		},
		{
			"Test gossamer --protocol",
			[]string{"config", "protocol"},
			[]interface{}{testCfgFile.Name(), "/gossamer/test/0"},
			dot.NetworkConfig{
				Port:        testCfg.Network.Port,
				Bootnodes:   testCfg.Network.Bootnodes,
				ProtocolID:  "/gossamer/test/0",
				NoBootstrap: testCfg.Network.NoBootstrap,
				NoMDNS:      testCfg.Network.NoMDNS,
			},
		},
		{
			"Test gossamer --nobootstrap",
			[]string{"config", "nobootstrap"},
			[]interface{}{testCfgFile.Name(), "true"},
			dot.NetworkConfig{
				Port:        testCfg.Network.Port,
				Bootnodes:   testCfg.Network.Bootnodes,
				ProtocolID:  testCfg.Network.ProtocolID,
				NoBootstrap: true,
				NoMDNS:      testCfg.Network.NoMDNS,
			},
		},
		{
			"Test gossamer --nomdns",
			[]string{"config", "nomdns"},
			[]interface{}{testCfgFile.Name(), "true"},
			dot.NetworkConfig{
				Port:        testCfg.Network.Port,
				Bootnodes:   testCfg.Network.Bootnodes,
				ProtocolID:  testCfg.Network.ProtocolID,
				NoBootstrap: testCfg.Network.NoBootstrap,
				NoMDNS:      true,
			},
		},
	}

	for _, c := range testcases {
		c := c // bypass scopelint false positive
		t.Run(c.description, func(t *testing.T) {
			ctx, err := newTestContext(c.description, c.flags, c.values)
			require.Nil(t, err)
			cfg, err := createDotConfig(ctx)
			require.Nil(t, err)
			require.Equal(t, c.expected, cfg.Network)
		})
	}
}

// TestRPCConfigFromFlags tests createDotRPCConfig using relevant rpc flags
func TestRPCConfigFromFlags(t *testing.T) {
	testCfg, testCfgFile := dot.NewTestConfigWithFile(t)
	require.NotNil(t, testCfg)
	require.NotNil(t, testCfgFile)

	defer utils.RemoveTestDir(t)

	testApp := cli.NewApp()
	testApp.Writer = ioutil.Discard

	testcases := []struct {
		description string
		flags       []string
		values      []interface{}
		expected    dot.RPCConfig
	}{
		{
			"Test gossamer --rpc",
			[]string{"config", "rpc"},
			[]interface{}{testCfgFile.Name(), "true"},
			dot.RPCConfig{
				Enabled: true,
				Port:    testCfg.RPC.Port,
				Host:    testCfg.RPC.Host,
				Modules: testCfg.RPC.Modules,
				WSPort:  testCfg.RPC.WSPort,
			},
		},
		{
			"Test gossamer --rpc false",
			[]string{"config", "rpc"},
			[]interface{}{testCfgFile.Name(), "false"},
			dot.RPCConfig{
				Enabled: false,
				Port:    testCfg.RPC.Port,
				Host:    testCfg.RPC.Host,
				Modules: testCfg.RPC.Modules,
				WSPort:  testCfg.RPC.WSPort,
			},
		},
		{
			"Test gossamer --rpchost",
			[]string{"config", "rpchost"},
			[]interface{}{testCfgFile.Name(), "testhost"}, // rpc must be enabled
			dot.RPCConfig{
				Enabled: testCfg.RPC.Enabled,
				Port:    testCfg.RPC.Port,
				Host:    "testhost",
				Modules: testCfg.RPC.Modules,
				WSPort:  testCfg.RPC.WSPort,
			},
		},
		{
			"Test gossamer --rpcport",
			[]string{"config", "rpcport"},
			[]interface{}{testCfgFile.Name(), "5678"}, // rpc must be enabled
			dot.RPCConfig{
				Enabled: testCfg.RPC.Enabled,
				Port:    5678,
				Host:    testCfg.RPC.Host,
				Modules: testCfg.RPC.Modules,
				WSPort:  testCfg.RPC.WSPort,
			},
		},
		{
			"Test gossamer --rpcsmods",
			[]string{"config", "rpcmods"},
			[]interface{}{testCfgFile.Name(), "mod1,mod2"}, // rpc must be enabled
			dot.RPCConfig{
				Enabled: testCfg.RPC.Enabled,
				Port:    testCfg.RPC.Port,
				Host:    testCfg.RPC.Host,
				Modules: []string{"mod1", "mod2"},
				WSPort:  testCfg.RPC.WSPort,
			},
		},
		{
			"Test gossamer --wsport",
			[]string{"config", "wsport"},
			[]interface{}{testCfgFile.Name(), "7070"},
			dot.RPCConfig{
				Enabled:   testCfg.RPC.Enabled,
				Port:      testCfg.RPC.Port,
				Host:      testCfg.RPC.Host,
				Modules:   testCfg.RPC.Modules,
				WSPort:    7070,
				WSEnabled: false,
			},
		},
		{
			"Test gossamer --ws",
			[]string{"config", "ws"},
			[]interface{}{testCfgFile.Name(), false},
			dot.RPCConfig{
				Enabled:   testCfg.RPC.Enabled,
				Port:      testCfg.RPC.Port,
				Host:      testCfg.RPC.Host,
				Modules:   testCfg.RPC.Modules,
				WSPort:    testCfg.RPC.WSPort,
				WSEnabled: false,
			},
		},
		{
			"Test gossamer --ws",
			[]string{"config", "ws"},
			[]interface{}{testCfgFile.Name(), true},
			dot.RPCConfig{
				Enabled:   testCfg.RPC.Enabled,
				Port:      testCfg.RPC.Port,
				Host:      testCfg.RPC.Host,
				Modules:   testCfg.RPC.Modules,
				WSPort:    testCfg.RPC.WSPort,
				WSEnabled: true,
			},
		},
	}

	for _, c := range testcases {
		c := c // bypass scopelint false positive
		t.Run(c.description, func(t *testing.T) {
			ctx, err := newTestContext(c.description, c.flags, c.values)
			require.Nil(t, err)
			cfg, err := createDotConfig(ctx)
			require.Nil(t, err)
			require.Equal(t, c.expected, cfg.RPC)
		})
	}
}

// TestUpdateConfigFromGenesisJSON tests updateDotConfigFromGenesisJSON
func TestUpdateConfigFromGenesisJSON(t *testing.T) {
	testCfg, testCfgFile := dot.NewTestConfigWithFile(t)
	genFile := dot.NewTestGenesisRawFile(t, testCfg)

	defer utils.RemoveTestDir(t)

	ctx, err := newTestContext(
		t.Name(),
		[]string{"config", "genesis-raw"},
		[]interface{}{testCfgFile.Name(), genFile.Name()},
	)
	require.Nil(t, err)

	expected := &dot.Config{
		Global: dot.GlobalConfig{
			Name:     testCfg.Global.Name,
			ID:       testCfg.Global.ID,
			BasePath: testCfg.Global.BasePath,
			LogLevel: testCfg.Global.LogLevel,
		},
		Log: dot.LogConfig{
			CoreLvl:           "info",
			SyncLvl:           "info",
			NetworkLvl:        "info",
			RPCLvl:            "info",
			StateLvl:          "info",
			RuntimeLvl:        "info",
			BlockProducerLvl:  "info",
			FinalityGadgetLvl: "info",
		},
		Init: dot.InitConfig{
			GenesisRaw:     genFile.Name(),
			TestFirstEpoch: true,
		},
		Account: testCfg.Account,
		Core:    testCfg.Core,
		Network: testCfg.Network,
		RPC:     testCfg.RPC,
		System:  testCfg.System,
	}

	cfg, err := createDotConfig(ctx)
	require.Nil(t, err)

	cfg.Init.GenesisRaw = genFile.Name()
	expected.Core.BabeThreshold = nil

	updateDotConfigFromGenesisJSONRaw(ctx, cfg)

	require.Equal(t, expected, cfg)
}

// TestUpdateConfigFromGenesisJSON_Default tests updateDotConfigFromGenesisJSON
// using the default genesis path if no genesis path is provided (ie, an empty
// genesis value provided in the toml configuration file or with --genesis "")
func TestUpdateConfigFromGenesisJSON_Default(t *testing.T) {
	testCfg, testCfgFile := dot.NewTestConfigWithFile(t)

	defer utils.RemoveTestDir(t)

	ctx, err := newTestContext(
		t.Name(),
		[]string{"config", "genesis-raw"},
		[]interface{}{testCfgFile.Name(), ""},
	)
	require.Nil(t, err)

	expected := &dot.Config{
		Global: dot.GlobalConfig{
			Name:     testCfg.Global.Name,
			ID:       testCfg.Global.ID,
			BasePath: testCfg.Global.BasePath,
			LogLevel: testCfg.Global.LogLevel,
		},
		Log: dot.LogConfig{
			CoreLvl:           "info",
			SyncLvl:           "info",
			NetworkLvl:        "info",
			RPCLvl:            "info",
			StateLvl:          "info",
			RuntimeLvl:        "info",
			BlockProducerLvl:  "info",
			FinalityGadgetLvl: "info",
		},
		Init: dot.InitConfig{
			GenesisRaw:     DefaultCfg.Init.GenesisRaw,
			TestFirstEpoch: true,
		},
		Account: testCfg.Account,
		Core:    testCfg.Core,
		Network: testCfg.Network,
		RPC:     testCfg.RPC,
		System:  testCfg.System,
	}

	expected.Core.BabeThreshold = nil

	cfg, err := createDotConfig(ctx)
	require.Nil(t, err)

	updateDotConfigFromGenesisJSONRaw(ctx, cfg)

	require.Equal(t, expected, cfg)
}

func TestUpdateConfigFromGenesisData(t *testing.T) {
	testCfg, testCfgFile := dot.NewTestConfigWithFile(t)
	genFile := dot.NewTestGenesisRawFile(t, testCfg)

	defer utils.RemoveTestDir(t)

	ctx, err := newTestContext(
		t.Name(),
		[]string{"config", "genesis"},
		[]interface{}{testCfgFile.Name(), genFile.Name()},
	)
	require.Nil(t, err)

	expected := &dot.Config{
		Global: dot.GlobalConfig{
			Name:     testCfg.Global.Name,
			ID:       testCfg.Global.ID,
			BasePath: testCfg.Global.BasePath,
			LogLevel: testCfg.Global.LogLevel,
		},
		Log: dot.LogConfig{
			CoreLvl:           "info",
			SyncLvl:           "info",
			NetworkLvl:        "info",
			RPCLvl:            "info",
			StateLvl:          "info",
			RuntimeLvl:        "info",
			BlockProducerLvl:  "info",
			FinalityGadgetLvl: "info",
		},
		Init: dot.InitConfig{
			GenesisRaw:     genFile.Name(),
			TestFirstEpoch: true,
		},
		Account: testCfg.Account,
		Core:    testCfg.Core,
		Network: dot.NetworkConfig{
			Port:        testCfg.Network.Port,
			Bootnodes:   []string{}, // TODO: improve cmd tests #687
			ProtocolID:  testCfg.Network.ProtocolID,
			NoBootstrap: testCfg.Network.NoBootstrap,
			NoMDNS:      testCfg.Network.NoMDNS,
		},
		RPC:    testCfg.RPC,
		System: testCfg.System,
	}

	cfg, err := createDotConfig(ctx)
	require.Nil(t, err)

	cfg.Init.GenesisRaw = genFile.Name()
	expected.Core.BabeThreshold = nil

	db, err := database.NewBadgerDB(cfg.Global.BasePath)
	require.Nil(t, err)

	gen, err := genesis.NewGenesisFromJSONRaw(genFile.Name())
	require.Nil(t, err)

	err = state.StoreGenesisData(db, gen.GenesisData())
	require.Nil(t, err)

	err = db.Close()
	require.Nil(t, err)

	err = updateDotConfigFromGenesisData(ctx, cfg) // name should not be updated if provided as flag value
	require.Nil(t, err)

	require.Equal(t, expected, cfg)
}
