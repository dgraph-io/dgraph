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
	"github.com/ChainSafe/gossamer/p2p"
	"github.com/ChainSafe/gossamer/polkadb"
)

var (
	defaultP2PPort = 7001
	defaultP2PRandSeed = int64(33)
)
// Config is a collection of configurations throughout the system
type Config struct {
	ServiceConfig *p2p.ServiceConfig
	DbConfig       polkadb.DbConfig
}

// DefaultConfig is the default settings used when a config.toml file is not passed in during instantiation
var DefaultConfig = &Config{
	ServiceConfig: &p2p.ServiceConfig{
		BootstrapNodes: []string{"/ip4/104.131.131.82/tcp/4001/ipfs/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ", "/ip4/104.236.179.241/tcp/4001/ipfs/QmSoLPppuBtQSGwKDZT2M73ULpjvfd3aZ6ha4oFGL1KrGM"},
		Port: defaultP2PPort,
		RandSeed: defaultP2PRandSeed,
	},
	DbConfig: polkadb.DbConfig{
		Datadir: "chaindata",
	},
}



//[ServiceConfig]
//BootstrapNodes=["/ip4/104.131.131.82/tcp/4001/ipfs/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ", "/ip4/104.236.179.241/tcp/4001/ipfs/QmSoLPppuBtQSGwKDZT2M73ULpjvfd3aZ6ha4oFGL1KrGM",]
//Port= 7001
//RandSeed= 33
//
//[DbConfig]
//Datadir="chaindata"




