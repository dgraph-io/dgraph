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

package ksmcc

var (
	// GlobalConfig

	// DefaultName Default node name
	DefaultName = string("ksmcc")
	// DefaultID Default chain ID
	DefaultID = string("ksmcc")
	// DefaultConfig Default toml configuration path
	DefaultConfig = string("./chain/ksmcc/config.toml")
	// DefaultBasePath Default node base directory path
	DefaultBasePath = string("~/.gossamer/ksmcc")

	// DefaultLvl is the default log level
	DefaultLvl = string("info")

	// InitConfig

	// DefaultGenesisRaw Default genesis configuration path
	DefaultGenesisRaw = string("./chain/ksmcc/genesis-raw.json")

	// AccountConfig

	// DefaultKey Default account key
	DefaultKey = string("")
	// DefaultUnlock Default account unlock
	DefaultUnlock = string("")

	// CoreConfig

	// DefaultAuthority true if BABE block producer
	DefaultAuthority = false
	// DefaultRoles Default node roles
	DefaultRoles = byte(1) // full node (see Table D.2)

	// NetworkConfig

	// DefaultNetworkPort network port
	DefaultNetworkPort = uint32(7001)
	// DefaultNetworkBootnodes network bootnodes
	DefaultNetworkBootnodes = []string(nil)
	// DefaultNetworkProtocolID network protocol
	DefaultNetworkProtocolID = string("/gossamer/ksmcc/0")
	// DefaultNoBootstrap disables bootstrap
	DefaultNoBootstrap = false
	// DefaultNoMDNS disables mDNS discovery
	DefaultNoMDNS = false

	// RPCConfig

	// DefaultRPCHTTPHost rpc host
	DefaultRPCHTTPHost = string("localhost")
	// DefaultRPCHTTPPort rpc port
	DefaultRPCHTTPPort = uint32(8545)
	// DefaultRPCModules rpc modules
	DefaultRPCModules = []string{"system"}
	// DefaultRPCWSPort rpc websocket port
	DefaultRPCWSPort = uint32(8546)
)
