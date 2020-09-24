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
	"github.com/ChainSafe/gossamer/lib/utils"

	log "github.com/ChainSafe/log15"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

// TestExportCommand test "gossamer export --config"
func TestExportCommand(t *testing.T) {
	testDir := utils.NewTestDir(t)
	testCfg, testConfigFile := newTestConfigWithFile(t)
	genFile := dot.NewTestGenesisRawFile(t, testCfg)

	defer utils.RemoveTestDir(t)

	testApp := cli.NewApp()
	testApp.Writer = ioutil.Discard

	testName := "testnode"
	testBootnode := "bootnode"
	testProtocol := "/protocol/test/0"
	testConfig := testConfigFile.Name()

	testcases := []struct {
		description string
		flags       []string
		values      []interface{}
		expected    *dot.Config
	}{
		{
			"Test gossamer export --config --genesis-raw --basepath --name --log --force",
			[]string{"config", "genesis-raw", "basepath", "name", "log", "force"},
			[]interface{}{testConfig, genFile.Name(), testDir, testName, log.LvlInfo.String(), "true"},
			&dot.Config{
				Global: dot.GlobalConfig{
					Name:     testName,
					ID:       testCfg.Global.ID,
					BasePath: testCfg.Global.BasePath,
					LogLvl:   log.LvlInfo,
				},
				Log: dot.LogConfig{
					CoreLvl:           log.LvlInfo,
					SyncLvl:           log.LvlInfo,
					NetworkLvl:        log.LvlInfo,
					RPCLvl:            log.LvlInfo,
					StateLvl:          log.LvlInfo,
					RuntimeLvl:        log.LvlInfo,
					BlockProducerLvl:  log.LvlInfo,
					FinalityGadgetLvl: log.LvlInfo,
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
			[]string{"config", "genesis-raw", "bootnodes", "name", "force"},
			[]interface{}{testConfig, genFile.Name(), testBootnode, "gssmr", "true"},
			&dot.Config{
				Global: testCfg.Global,
				Init: dot.InitConfig{
					GenesisRaw: genFile.Name(),
				},
				Log: dot.LogConfig{
					CoreLvl:           log.LvlInfo,
					SyncLvl:           log.LvlInfo,
					NetworkLvl:        log.LvlInfo,
					RPCLvl:            log.LvlInfo,
					StateLvl:          log.LvlInfo,
					RuntimeLvl:        log.LvlInfo,
					BlockProducerLvl:  log.LvlInfo,
					FinalityGadgetLvl: log.LvlInfo,
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
			[]string{"config", "genesis-raw", "protocol", "force"},
			[]interface{}{testConfig, genFile.Name(), testProtocol, "true"},
			&dot.Config{
				Global: testCfg.Global,
				Init: dot.InitConfig{
					GenesisRaw: genFile.Name(),
				},
				Log: dot.LogConfig{
					CoreLvl:           log.LvlInfo,
					SyncLvl:           log.LvlInfo,
					NetworkLvl:        log.LvlInfo,
					RPCLvl:            log.LvlInfo,
					StateLvl:          log.LvlInfo,
					RuntimeLvl:        log.LvlInfo,
					BlockProducerLvl:  log.LvlInfo,
					FinalityGadgetLvl: log.LvlInfo,
				},
				Account: testCfg.Account,
				Core:    testCfg.Core,
				Network: dot.NetworkConfig{
					Port:        testCfg.Network.Port,
					Bootnodes:   []string{testBootnode}, // TODO: improve cmd tests #687
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

			cfg := new(Config)
			err = loadConfig(cfg, config)
			require.Nil(t, err)

			require.Equal(t, dotConfigToToml(c.expected), cfg)
		})
	}
}
