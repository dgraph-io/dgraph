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

package dot

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/ChainSafe/gossamer/lib/genesis"
	"github.com/ChainSafe/gossamer/lib/utils"

	"github.com/stretchr/testify/require"
)

// NewTestConfig returns a new test configuration using the provided datadir
func NewTestConfig(t *testing.T) *Config {
	dir := utils.NewTestDir(t)

	return &Config{
		Global: GlobalConfig{
			Name:    string("test"),
			ID:      string("test"),
			DataDir: dir,
			Config:  string(""),
			Genesis: string(""),
			Roles:   byte(4), // authority node
		},
		Account: AccountConfig{
			Key:    string(""),
			Unlock: string(""),
		},
		Core: CoreConfig{
			Authority: true, // BABE block producer
		},
		Network: NetworkConfig{
			Port:        uint32(7001),
			Bootnodes:   []string(nil),
			ProtocolID:  string("/gossamer/test/0"),
			NoBootstrap: false,
			NoMDNS:      false,
		},
		RPC: RPCConfig{
			Host:    string("localhost"),
			Port:    uint32(8545),
			Modules: []string{"system", "author"},
		},
	}
}

// NewTestConfigWithFile returns a new test configuration and a temporary configuration file
func NewTestConfigWithFile(t *testing.T) (*Config, *os.File) {
	cfg := NewTestConfig(t)

	file, err := ioutil.TempFile(cfg.Global.DataDir, "config-")
	if err != nil {
		fmt.Println(fmt.Errorf("failed to create temporary file: %s", err))
		require.Nil(t, err)
	}

	cfgFile := ExportConfig(cfg, file.Name())

	return cfg, cfgFile
}

// NewTestGenesis returns a test genesis instance using "gssmr" raw data
func NewTestGenesis(t *testing.T) *genesis.Genesis {
	gssmrGen, err := genesis.LoadGenesisFromJSON("../node/gssmr/genesis.json")
	if err != nil {
		t.Fatal(err)
	}
	return &genesis.Genesis{
		Name:       "test",
		ID:         "test",
		Bootnodes:  []string(nil),
		ProtocolID: "/gossamer/test/0",
		Genesis:    gssmrGen.GenesisFields(),
	}
}

// NewTestGenesisFile returns a test genesis file using "gssmr" raw data
func NewTestGenesisFile(t *testing.T, cfg *Config) *os.File {
	dir := utils.NewTestDir(t)

	file, err := ioutil.TempFile(dir, "genesis-")
	if err != nil {
		fmt.Println(fmt.Errorf("failed to create temporary file: %s", err))
		require.Nil(t, err)
	}

	gssmrGen, err := genesis.LoadGenesisFromJSON("../node/gssmr/genesis.json")
	if err != nil {
		t.Fatal(err)
	}

	gen := &genesis.Genesis{
		Name:       cfg.Global.Name,
		ID:         cfg.Global.ID,
		Bootnodes:  cfg.Network.Bootnodes,
		ProtocolID: cfg.Network.ProtocolID,
		Genesis:    gssmrGen.GenesisFields(),
	}

	b, err := json.Marshal(gen)
	require.Nil(t, err)

	_, err = file.Write(b)
	require.Nil(t, err)

	return file
}
