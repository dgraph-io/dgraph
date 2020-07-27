// Copyright 2020 ChainSafe Systems (ON) Corp.
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
	"os"
	"testing"

	"github.com/ChainSafe/gossamer/lib/genesis"
	"github.com/stretchr/testify/require"
)

func TestBuildFromGenesis(t *testing.T) {
	file, err := genesis.CreateTestGenesisJSONFile(false)
	defer os.Remove(file)
	require.NoError(t, err)
	bs, err := BuildFromGenesis(file)
	require.NoError(t, err)

	// confirm human-readable fields
	hr, err := bs.ToJSON()
	require.NoError(t, err)
	jGen := genesis.Genesis{}
	err = json.Unmarshal(hr, &jGen)
	require.NoError(t, err)
	genesis.TestGenesis.Genesis = genesis.TestFieldsHR
	require.Equal(t, genesis.TestGenesis.Genesis.Runtime, jGen.Genesis.Runtime)

	// confirm raw fields
	raw, err := bs.ToJSONRaw()
	require.NoError(t, err)
	jGenRaw := genesis.Genesis{}
	err = json.Unmarshal(raw, &jGenRaw)
	require.NoError(t, err)
	genesis.TestGenesis.Genesis = genesis.TestFieldsRaw
	require.Equal(t, genesis.TestGenesis.Genesis.Raw, jGenRaw.Genesis.Raw)
}

func TestBuildFromDB(t *testing.T) {
	// setup expected
	cfg := NewTestConfig(t)
	cfg.Init.GenesisRaw = "../chain/gssmr/genesis-raw.json"
	expected, err := genesis.NewGenesisFromJSONRaw(cfg.Init.GenesisRaw)
	require.NoError(t, err)

	// initialize node (initialize state database and load genesis data)
	err = InitNode(cfg)
	require.NoError(t, err)

	bs, err := BuildFromDB(cfg.Global.BasePath)
	require.NoError(t, err)
	res, err := bs.ToJSON()
	require.NoError(t, err)
	jGen := genesis.Genesis{}
	err = json.Unmarshal(res, &jGen)
	require.NoError(t, err)

	require.Equal(t, expected.Genesis.Raw[0]["0x3a636f6465"], jGen.Genesis.Runtime["system"]["code"])
}
