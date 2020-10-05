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
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/ChainSafe/gossamer/lib/genesis"
	"github.com/ChainSafe/gossamer/lib/runtime/wasmer"
	"github.com/ChainSafe/gossamer/lib/utils"
	log "github.com/ChainSafe/log15"
	"github.com/naoina/toml"
	"github.com/stretchr/testify/require"
)

// setupLogger sets up the gossamer logger
func setupLogger(cfg *Config) {
	handler := log.StreamHandler(os.Stdout, log.TerminalFormat())
	handler = log.CallerFileHandler(handler)
	logger.SetHandler(log.LvlFilterHandler(cfg.Global.LogLvl, handler))
}

// NewTestGenesis returns a test genesis instance using "gssmr" raw data
func NewTestGenesis(t *testing.T) *genesis.Genesis {
	fp := utils.GetGssmrGenesisRawPath()

	gssmrGen, err := genesis.NewGenesisFromJSONRaw(fp)
	require.NoError(t, err)

	return &genesis.Genesis{
		Name:       "test",
		ID:         "test",
		Bootnodes:  []string(nil),
		ProtocolID: "/gossamer/test/0",
		Genesis:    gssmrGen.GenesisFields(),
	}
}

// NewTestGenesisRawFile returns a test genesis-raw file using "gssmr" raw data
func NewTestGenesisRawFile(t *testing.T, cfg *Config) *os.File {
	dir := utils.NewTestDir(t)

	file, err := ioutil.TempFile(dir, "genesis-")
	require.Nil(t, err)

	fp := utils.GetGssmrGenesisRawPath()

	gssmrGen, err := genesis.NewGenesisFromJSONRaw(fp)
	require.Nil(t, err)

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

// NewTestGenesisFile returns a human-readable test genesis file using "gssmr" human readable data
func NewTestGenesisFile(t *testing.T, cfg *Config) *os.File {
	dir := utils.NewTestDir(t)

	file, err := ioutil.TempFile(dir, "genesis-")
	require.Nil(t, err)

	fp := utils.GetGssmrGenesisPath()

	gssmrGen, err := genesis.NewGenesisFromJSON(fp)
	require.Nil(t, err)

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

// NewTestGenesisAndRuntime create a new test runtime and a new test genesis
// file with the test runtime stored in raw data and returns the genesis file
// nolint
func NewTestGenesisAndRuntime(t *testing.T) string {
	dir := utils.NewTestDir(t)

	_ = wasmer.NewTestInstance(t, wasmer.NODE_RUNTIME)
	runtimeFilePath := wasmer.GetAbsolutePath(wasmer.NODE_RUNTIME_FP)

	runtimeData, err := ioutil.ReadFile(runtimeFilePath)
	require.Nil(t, err)

	gen := NewTestGenesis(t)
	hex := hex.EncodeToString(runtimeData)

	gen.Genesis.Raw = [2]map[string]string{}
	if gen.Genesis.Raw[0] == nil {
		gen.Genesis.Raw[0] = make(map[string]string)
	}
	gen.Genesis.Raw[0]["0x3a636f6465"] = "0x" + hex
	gen.Genesis.Raw[0]["0xcf722c0832b5231d35e29f319ff27389f5032bfc7bfc3ba5ed7839f2042fb99f"] = "0x0000000000000001"

	genFile, err := ioutil.TempFile(dir, "genesis-")
	require.Nil(t, err)

	genData, err := json.Marshal(gen)
	require.Nil(t, err)

	_, err = genFile.Write(genData)
	require.Nil(t, err)

	return genFile.Name()
}

// NewTestConfig returns a new test configuration using the provided basepath
func NewTestConfig(t *testing.T) *Config {
	dir := utils.NewTestDir(t)

	// TODO: use default config instead of gssmr config for test config #776

	cfg := &Config{
		Global: GlobalConfig{
			Name:     GssmrConfig().Global.Name,
			ID:       GssmrConfig().Global.ID,
			BasePath: dir,
			LogLvl:   log.LvlInfo,
		},
		Log:     GssmrConfig().Log,
		Init:    GssmrConfig().Init,
		Account: GssmrConfig().Account,
		Core:    GssmrConfig().Core,
		Network: GssmrConfig().Network,
		RPC:     GssmrConfig().RPC,
		System:  GssmrConfig().System,
	}

	cfg.Init.TestFirstEpoch = true
	return cfg
}

// NewTestConfigWithFile returns a new test configuration and a temporary configuration file
func NewTestConfigWithFile(t *testing.T) (*Config, *os.File) {
	cfg := NewTestConfig(t)

	file, err := ioutil.TempFile(cfg.Global.BasePath, "config-")
	require.NoError(t, err)

	cfgFile := ExportConfig(cfg, file.Name())
	return cfg, cfgFile
}

// ExportConfig exports a dot configuration to a toml configuration file
func ExportConfig(cfg *Config, fp string) *os.File {
	var (
		newFile *os.File
		err     error
		raw     []byte
	)

	if raw, err = toml.Marshal(*cfg); err != nil {
		logger.Error("failed to marshal configuration", "error", err)
		os.Exit(1)
	}

	newFile, err = os.Create(filepath.Clean(fp))
	if err != nil {
		logger.Error("failed to create configuration file", "error", err)
		os.Exit(1)
	}

	_, err = newFile.Write(raw)
	if err != nil {
		logger.Error("failed to write to configuration file", "error", err)
		os.Exit(1)
	}

	if err := newFile.Close(); err != nil {
		logger.Error("failed to close configuration file", "error", err)
		os.Exit(1)
	}

	return newFile
}
