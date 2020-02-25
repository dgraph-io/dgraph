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

package node

import (
	"encoding/json"
	"os"

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

// String will return the json representation for a Config
func (c *Config) String() string {
	out, _ := json.MarshalIndent(c, "", "\t")
	return string(out)
}

// ExportConfig encodes a state type into a TOML file.
func ExportConfig(file string, s *Config) *os.File {
	var (
		newFile *os.File
		err     error
	)

	var raw []byte
	if raw, err = toml.Marshal(*s); err != nil {
		log.Warn("error marshaling toml", "err", err)
		os.Exit(1)
	}

	newFile, err = os.Create(file)
	if err != nil {
		log.Warn("error creating config file", "err", err)
	}
	_, err = newFile.Write(raw)
	if err != nil {
		log.Warn("error writing to config file", "err", err)
	}

	if err := newFile.Close(); err != nil {
		log.Warn("error closing file", "err", err)
	}
	return newFile
}
