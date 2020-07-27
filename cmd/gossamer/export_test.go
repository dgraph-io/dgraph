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
	"path"
	"testing"

	"github.com/ChainSafe/gossamer/dot"
	"github.com/ChainSafe/gossamer/lib/utils"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

// TestExportCommand test "gossamer export --config"
func TestExportCommand(t *testing.T) {
	testDir := utils.NewTestDir(t)
	testCfg := dot.NewTestConfig(t)

	genFile := dot.NewTestGenesisRawFile(t, testCfg)

	defer utils.RemoveTestDir(t)

	testApp := cli.NewApp()
	testApp.Writer = ioutil.Discard

	testName := "testnode"
	testBootnode := "bootnode"
	testProtocol := "/protocol/test/0"
	testConfig := path.Join(testDir, "config.toml")

	testcases := []struct {
		description string
		flags       []string
		values      []interface{}
		expected    *dot.Config
	}{
		{
			"Test gossamer export --config --genesis-raw --basepath --name --log",
			[]string{"config", "genesis-raw", "basepath", "name", "log"},
			[]interface{}{testConfig, genFile.Name(), testDir, testName, "info"},
			&dot.Config{
				Global: dot.GlobalConfig{
					Name:     testName,
					ID:       testCfg.Global.ID,
					BasePath: testCfg.Global.BasePath,
					LogLevel: "info",
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
					GenesisRaw: genFile.Name(),
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
				RPC: testCfg.RPC,
			},
		},
		{
			"Test gossamer export --config --genesis-raw --bootnodes --log --force",
			[]string{"config", "genesis-raw", "bootnodes", "log", "force"},
			[]interface{}{testConfig, genFile.Name(), testBootnode, "info", "true"},
			&dot.Config{
				Global: testCfg.Global,
				Init: dot.InitConfig{
					GenesisRaw: genFile.Name(),
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
				Account: testCfg.Account,
				Core:    testCfg.Core,
				Network: dot.NetworkConfig{
					Port:        testCfg.Network.Port,
					Bootnodes:   []string{testBootnode},
					ProtocolID:  testCfg.Network.ProtocolID,
					NoBootstrap: testCfg.Network.NoBootstrap,
					NoMDNS:      testCfg.Network.NoMDNS,
				},
				RPC: testCfg.RPC,
			},
		},
		{
			"Test gossamer export --config --genesis-raw --protocol --log --force",
			[]string{"config", "genesis-raw", "protocol", "log", "force"},
			[]interface{}{testConfig, genFile.Name(), testProtocol, "info", "true"},
			&dot.Config{
				Global: testCfg.Global,
				Init: dot.InitConfig{
					GenesisRaw: genFile.Name(),
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
				Account: testCfg.Account,
				Core:    testCfg.Core,
				Network: dot.NetworkConfig{
					Port:        testCfg.Network.Port,
					Bootnodes:   []string{}, // TODO: improve cmd tests #687
					ProtocolID:  testProtocol,
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

			err = exportAction(ctx)
			require.Nil(t, err)

			config := ctx.GlobalString(ConfigFlag.Name)

			cfg := new(dot.Config)
			err = dot.LoadConfig(cfg, config)
			require.Nil(t, err)

			require.Equal(t, c.expected, cfg)
		})
	}
}
