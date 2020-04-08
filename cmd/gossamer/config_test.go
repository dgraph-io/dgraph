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

	"github.com/ChainSafe/gossamer/dot"
	"github.com/ChainSafe/gossamer/lib/genesis"
	"github.com/ChainSafe/gossamer/lib/utils"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

// TODO: TestSetDotGlobalConfig - add cmd config tests

// TODO: TestSetDotAccountConfig - add cmd config tests

// TODO: TestSetDotCoreConfig - add cmd config tests

// TODO: TestSetDotNetworkConfig - add cmd config tests

// TODO: TestSetDotRPCConfig - add cmd config tests

// TestConfigFromNodeFlag tests createDotConfig using the --node flag
func TestConfigFromNodeFlag(t *testing.T) {
	testApp := cli.NewApp()
	testApp.Writer = ioutil.Discard

	testcases := []struct {
		description string
		flags       []string
		values      []interface{}
		expected    *dot.Config
	}{
		{
			"Test gossamer --node gssmr",
			[]string{"node"},
			[]interface{}{"gssmr"},
			dot.GssmrConfig(),
		},
		{
			"Test gossamer --node ksmcc",
			[]string{"node"},
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
			"Test gossamer --genesis",
			[]string{"config", "genesis"},
			[]interface{}{testCfgFile.Name(), "test_genesis"},
			dot.InitConfig{
				Genesis: "test_genesis",
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
				Name:    testCfg.Global.Name,
				ID:      testCfg.Global.ID,
				DataDir: testCfg.Global.DataDir,
			},
		},
		{
			"Test gossamer --node",
			[]string{"config", "node"},
			[]interface{}{testCfgFile.Name(), "ksmcc"},
			dot.GlobalConfig{
				Name:    testCfg.Global.Name,
				ID:      "ksmcc",
				DataDir: testCfg.Global.DataDir,
			},
		},
		{
			"Test gossamer --node",
			[]string{"config", "name"},
			[]interface{}{testCfgFile.Name(), "test_name"},
			dot.GlobalConfig{
				Name:    "test_name",
				ID:      testCfg.Global.ID,
				DataDir: testCfg.Global.DataDir,
			},
		},
		{
			"Test gossamer --datadir",
			[]string{"config", "datadir"},
			[]interface{}{testCfgFile.Name(), "test_datadir"},
			dot.GlobalConfig{
				Name:    testCfg.Global.Name,
				ID:      testCfg.Global.ID,
				DataDir: "test_datadir",
			},
		},
		{
			"Test gossamer --roles",
			[]string{"config", "roles"},
			[]interface{}{testCfgFile.Name(), "1"},
			dot.GlobalConfig{
				Name:    testCfg.Global.Name,
				ID:      testCfg.Global.ID,
				DataDir: testCfg.Global.DataDir,
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
				Authority: true,
				Roles:     4,
			},
		},
		{
			"Test gossamer --roles",
			[]string{"config", "roles"},
			[]interface{}{testCfgFile.Name(), "0"},
			dot.CoreConfig{
				Authority: false,
				Roles:     0,
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
			[]interface{}{testCfgFile.Name(), uint(1234)},
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
			[]interface{}{testCfgFile.Name(), true},
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
			[]interface{}{testCfgFile.Name(), true},
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
			[]interface{}{testCfgFile.Name(), true},
			dot.RPCConfig{
				Enabled: true,
				Port:    testCfg.RPC.Port,
				Host:    testCfg.RPC.Host,
				Modules: testCfg.RPC.Modules,
			},
		},
		{
			"Test gossamer --rpc false",
			[]string{"config", "rpc"},
			[]interface{}{testCfgFile.Name(), false},
			dot.RPCConfig{
				Enabled: false,
				Port:    testCfg.RPC.Port,
				Host:    testCfg.RPC.Host,
				Modules: testCfg.RPC.Modules,
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
			},
		},
		{
			"Test gossamer --rpcport",
			[]string{"config", "rpcport"},
			[]interface{}{testCfgFile.Name(), uint(5678)}, // rpc must be enabled
			dot.RPCConfig{
				Enabled: testCfg.RPC.Enabled,
				Port:    5678,
				Host:    testCfg.RPC.Host,
				Modules: testCfg.RPC.Modules,
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

// TestUpdateConfigFromGenesis tests updateDotConfigFromGenesis
func TestUpdateConfigFromGenesis(t *testing.T) {
	testCfg, testCfgFile := dot.NewTestConfigWithFile(t)
	require.NotNil(t, testCfg)
	require.NotNil(t, testCfgFile)

	testGenFile := dot.NewTestGenesisFile(t, testCfg)

	defer utils.RemoveTestDir(t)

	testGen, err := genesis.NewGenesisFromJSON(testGenFile.Name())
	require.Nil(t, err)

	testApp := cli.NewApp()
	testApp.Writer = ioutil.Discard

	testcases := []struct {
		description string
		flags       []string
		values      []interface{}
		expected    dot.Config
	}{
		{
			"Test gossamer --genesis",
			[]string{"config", "genesis"},
			[]interface{}{testCfgFile.Name(), testGenFile.Name()},
			dot.Config{
				Global: dot.GlobalConfig{
					Name:    testGen.Name, // genesis name
					ID:      testGen.ID,   // genesis id
					DataDir: testCfg.Global.DataDir,
				},
				Account: testCfg.Account,
				Core:    testCfg.Core,
				Network: dot.NetworkConfig{
					Port:        testCfg.Network.Port,
					Bootnodes:   testGen.Bootnodes,  // genesis bootnodes
					ProtocolID:  testGen.ProtocolID, // genesis protocol id
					NoBootstrap: testCfg.Network.NoBootstrap,
					NoMDNS:      testCfg.Network.NoMDNS,
				},
				RPC: testCfg.RPC,
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

			require.Equal(t, &c.expected, cfg)
		})
	}
}
