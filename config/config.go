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

package cfg

import (
	"bytes"
	"os"

	tml "github.com/BurntSushi/toml"
	"github.com/ChainSafe/gossamer/internal/api"
	"github.com/ChainSafe/gossamer/polkadb"
	log "github.com/ChainSafe/log15"
)

// Config is a collection of configurations throughout the system
type Config struct {
	P2p   P2pCfg         `toml:"p2p"`
	DbCfg polkadb.Config `toml:"db"`
	Rpc   RpcCfg         `toml:"rpc"`
}

type P2pCfg struct {
	BootstrapNodes []string `toml:"bootstrap-nodes"`
	Port           uint32   `toml:"port"`
	NoBootstrap    bool     `toml:"no-bootstrap"`
	NoMdns         bool     `toml:"no-mdns"`
}

type RpcCfg struct {
	Port    uint32       `toml:"port"`
	Host    string       `toml:"host"`
	Modules []api.Module `toml:"modules"`
}

// ToTOML encodes a state type into a TOML file.
func ToTOML(file string, s *Config) *os.File {
	var buff bytes.Buffer
	var (
		newFile *os.File
		err     error
	)

	if err = tml.NewEncoder(&buff).Encode(s); err != nil {
		log.Warn("error closing file", "err", err)
		os.Exit(1)
	}

	newFile, err = os.Create(file)
	if err != nil {
		log.Warn("error closing file", "err", err)
	}
	_, err = newFile.Write([]byte(buff.Bytes()))
	if err != nil {
		log.Warn("error closing file", "err", err)
	}

	if err := newFile.Close(); err != nil {
		log.Warn("error closing file", "err", err)
	}
	return newFile
}
