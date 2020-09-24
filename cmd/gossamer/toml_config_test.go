package main

import (
	"testing"

	"github.com/ChainSafe/gossamer/dot"
	"github.com/ChainSafe/gossamer/lib/utils"

	"github.com/stretchr/testify/require"
)

const GssmrConfigPath = "../../chain/gssmr/config.toml"
const GssmrGenesisPath = "../../chain/gssmr/genesis-raw.json"

const KsmccConfigPath = "../../chain/ksmcc/config.toml"
const KsmccGenesisPath = "../../chain/ksmcc/genesis-raw.json"

// TestLoadConfig tests loading a toml configuration file
func TestLoadConfig(t *testing.T) {
	cfg, cfgFile := newTestConfigWithFile(t)
	require.NotNil(t, cfg)

	genFile := dot.NewTestGenesisRawFile(t, cfg)
	require.NotNil(t, genFile)

	defer utils.RemoveTestDir(t)

	cfg.Init.GenesisRaw = genFile.Name()

	err := dot.InitNode(cfg)
	require.Nil(t, err)

	err = loadConfig(dotConfigToToml(cfg), cfgFile.Name())
	require.Nil(t, err)
	require.NotNil(t, cfg)
}

// TestLoadConfigGssmr tests loading the toml configuration file for gssmr
func TestLoadConfigGssmr(t *testing.T) {
	cfg := dot.GssmrConfig()
	require.NotNil(t, cfg)

	cfg.Global.BasePath = utils.NewTestDir(t)
	cfg.Init.GenesisRaw = GssmrGenesisPath
	cfg.Init.TestFirstEpoch = true

	defer utils.RemoveTestDir(t)

	err := dot.InitNode(cfg)
	require.Nil(t, err)

	err = loadConfig(dotConfigToToml(cfg), GssmrConfigPath)
	require.Nil(t, err)
	require.NotNil(t, cfg)
}

// TestLoadConfigKsmcc tests loading the toml configuration file for ksmcc
func TestLoadConfigKsmcc(t *testing.T) {
	cfg := dot.KsmccConfig()
	require.NotNil(t, cfg)

	cfg.Global.BasePath = utils.NewTestDir(t)
	cfg.Init.GenesisRaw = KsmccGenesisPath
	cfg.Init.TestFirstEpoch = true

	defer utils.RemoveTestDir(t)

	err := dot.InitNode(cfg)
	require.Nil(t, err)

	err = loadConfig(dotConfigToToml(cfg), KsmccConfigPath)
	require.Nil(t, err)
}
