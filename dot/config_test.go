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
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
)

// NewTestConfig returns a new test configuration using the provided datadir
func NewTestConfig(datadir string) *Config {
	return &Config{
		Global: GlobalConfig{
			DataDir:   datadir,
			Roles:     byte(1),
			Authority: false,
		},
		Network: NetworkConfig{
			Bootnodes:   []string{},
			ProtocolID:  "/gossamer/test/0",
			Port:        7001,
			NoBootstrap: false,
			NoMDNS:      false,
		},
		RPC: RPCConfig{
			Host:    "localhost",
			Port:    8545,
			Modules: []string{"system"},
		},
	}
}

// TestLoadConfig tests loading toml configuration file for ksmcc
func TestLoadConfig(t *testing.T) {
	dir := path.Join(os.TempDir(), "gossamer-test", t.Name())
	defer os.RemoveAll(dir)

	fp := "../tests/exports/config.toml"
	cfg := NewTestConfig(dir)

	err := LoadConfig(fp, cfg)
	require.Nil(t, err)
}

// TestExportConfig tests exporting toml configuration file
func TestExportConfig(t *testing.T) {
	dir := path.Join(os.TempDir(), "gossamer-test", t.Name())
	defer os.RemoveAll(dir)

	fp := "../tests/exports/config.toml"
	cfg := NewTestConfig(dir)

	file := ExportConfig(fp, cfg)
	require.NotNil(t, file)
}
