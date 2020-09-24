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
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"unicode"

	"github.com/naoina/toml"
)

// Config is a collection of configurations throughout the system
type Config struct {
	Global  GlobalConfig  `toml:"global"`
	Log     LogConfig     `toml:"log"`
	Init    InitConfig    `toml:"init"`
	Account AccountConfig `toml:"account"`
	Core    CoreConfig    `toml:"core"`
	Network NetworkConfig `toml:"network"`
	RPC     RPCConfig     `toml:"rpc"`
}

// GlobalConfig is to marshal/unmarshal toml global config vars
type GlobalConfig struct {
	Name     string `toml:"name"`
	ID       string `toml:"id"`
	BasePath string `toml:"basepath"`
	LogLvl   string `toml:"log"`
}

// LogConfig represents the log levels for individual packages
type LogConfig struct {
	CoreLvl           string `toml:"core"`
	SyncLvl           string `toml:"sync"`
	NetworkLvl        string `toml:"network"`
	RPCLvl            string `toml:"rpc"`
	StateLvl          string `toml:"state"`
	RuntimeLvl        string `toml:"runtime"`
	BlockProducerLvl  string `toml:"babe"`
	FinalityGadgetLvl string `toml:"grandpa"`
}

// InitConfig is the configuration for the node initialization
type InitConfig struct {
	GenesisRaw string `toml:"genesis-raw"`
}

// AccountConfig is to marshal/unmarshal account config vars
type AccountConfig struct {
	Key    string `toml:"key"`
	Unlock string `toml:"unlock"`
}

// NetworkConfig is to marshal/unmarshal toml network config vars
type NetworkConfig struct {
	Port        uint32   `toml:"port"`
	Bootnodes   []string `toml:"bootnodes"`
	ProtocolID  string   `toml:"protocol"`
	NoBootstrap bool     `toml:"nobootstrap"`
	NoMDNS      bool     `toml:"nomdns"`
}

// CoreConfig is to marshal/unmarshal toml core config vars
type CoreConfig struct {
	Roles            byte   `toml:"roles"`
	BabeAuthority    bool   `toml:"babe-authority"`
	GrandpaAuthority bool   `toml:"grandpa-authority"`
	BabeThreshold    string `toml:"babe-threshold"`
	SlotDuration     uint64 `toml:"slot-duration"`
}

// RPCConfig is to marshal/unmarshal toml RPC config vars
type RPCConfig struct {
	Enabled   bool     `toml:"enabled"`
	Port      uint32   `toml:"port"`
	Host      string   `toml:"host"`
	Modules   []string `toml:"modules"`
	WSPort    uint32   `toml:"ws-port"`
	WSEnabled bool     `toml:"ws-enabled"`
}

// loadConfig loads the values from the toml configuration file into the provided configuration
func loadConfig(cfg *Config, fp string) error {
	fp, err := filepath.Abs(fp)
	if err != nil {
		logger.Error("failed to create absolute path for toml configuration file", "error", err)
		return err
	}

	file, err := os.Open(filepath.Clean(fp))
	if err != nil {
		logger.Error("failed to open toml configuration file", "error", err)
		return err
	}

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

	if err = tomlSettings.NewDecoder(file).Decode(&cfg); err != nil {
		logger.Error("failed to decode configuration", "error", err)
		return err
	}

	return nil
}

// exportConfig exports a dot configuration to a toml configuration file
func exportConfig(cfg *Config, fp string) *os.File {
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
