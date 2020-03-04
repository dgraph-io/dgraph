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
	"os"
	"path/filepath"
	"reflect"
	"unicode"

	log "github.com/ChainSafe/log15"
	"github.com/naoina/toml"
)

// Config is a collection of configurations throughout the system
type Config struct {
	Global  GlobalConfig  `toml:"global"`
	Network NetworkConfig `toml:"network"`
	RPC     RPCConfig     `toml:"rpc"`
}

// GlobalConfig is to marshal/unmarshal toml global config vars
type GlobalConfig struct {
	DataDir   string `toml:"data-dir"`
	Roles     byte   `toml:"roles"`
	Authority bool   `toml:"authority"`
}

// NetworkConfig is to marshal/unmarshal toml p2p vars
type NetworkConfig struct {
	Bootnodes   []string `toml:"bootstrap-nodes"`
	ProtocolID  string   `toml:"protocol-id"`
	Port        uint32   `toml:"port"`
	NoBootstrap bool     `toml:"no-bootstrap"`
	NoMdns      bool     `toml:"no-mdns"`
}

// RPCConfig is to marshal/unmarshal toml RPC vars
type RPCConfig struct {
	Port    uint32   `toml:"port"`
	Host    string   `toml:"host"`
	Modules []string `toml:"modules"`
}

// These settings ensure that TOML keys use the same names as Go struct fields.
var tomlSettings = toml.Config{
	NormFieldName: func(rt reflect.Type, key string) string {
		return key
	},
	FieldToKey: func(rt reflect.Type, field string) string {
		return field
	},
	MissingField: func(rt reflect.Type, field string) error {
		link := ""
		if unicode.IsUpper(rune(rt.Name()[0])) && rt.PkgPath() != "main" {
			link = fmt.Sprintf(", see https://godoc.org/%s#%s for available fields", rt.PkgPath(), rt.Name())
		}
		return fmt.Errorf("field '%s' is not defined in %s%s", field, rt.String(), link)
	},
}

// String will return the json representation for a Config
func (c *Config) String() string {
	out, _ := json.MarshalIndent(c, "", "\t")
	return string(out)
}

// LoadConfig loads the contents from config toml and inits Config object
func LoadConfig(fp string, cfg *Config) error {
	fp, err := filepath.Abs(fp)
	if err != nil {
		log.Error("[dot] Failed to create absolute filepath", "error", err)
		return err
	}

	file, err := os.Open(filepath.Clean(fp))
	if err != nil {
		log.Error("[dot] Failed to open file", "error", err)
		return err
	}

	if err = tomlSettings.NewDecoder(file).Decode(&cfg); err != nil {
		log.Error("[dot] Failed to decode configuration", "error", err)
		return err
	}

	return nil
}

// ExportConfig encodes a state type into a TOML file.
func ExportConfig(fp string, cfg *Config) *os.File {
	var (
		newFile *os.File
		err     error
		raw     []byte
	)

	if raw, err = toml.Marshal(*cfg); err != nil {
		log.Error("[dot] Failed to marshal configuration", "error", err)
		os.Exit(1)
	}

	newFile, err = os.Create(filepath.Clean(fp))
	if err != nil {
		log.Error("[dot] Failed to create configuration file", "error", err)
		os.Exit(1)
	}

	_, err = newFile.Write(raw)
	if err != nil {
		log.Error("[dot] Failed to write to configuration file", "error", err)
		os.Exit(1)
	}

	if err := newFile.Close(); err != nil {
		log.Error("[dot] Failed to close configuration file", "error", err)
		os.Exit(1)
	}

	return newFile
}
