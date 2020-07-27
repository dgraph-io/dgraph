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

package gssmr

var (
	// GlobalConfig

	// DefaultName Default node name
	DefaultName = string("gssmr")
	// DefaultID Default chain ID
	DefaultID = string("gssmr")
	// DefaultConfig Default toml configuration path
	DefaultConfig = string("./chain/gssmr/config.toml")
	// DefaultBasePath Default node base directory path
	DefaultBasePath = string("~/.gossamer/gssmr")

	// DefaultLvl is the default log level
	DefaultLvl = string("info")

	// InitConfig

	// DefaultGenesisRaw Default genesis configuration path
	DefaultGenesisRaw = string("./chain/gssmr/genesis-raw.json")

	// AccountConfig

	// DefaultKey Default account key
	DefaultKey = string("")
	// DefaultUnlock Default account unlock
	DefaultUnlock = string("")

	// CoreConfig

	// DefaultAuthority is true if the node is a block producer and a grandpa authority
	DefaultAuthority = true
	// DefaultRoles Default node roles
	DefaultRoles = byte(4) // authority node (see Table D.2)
	// DefaultBabeAuthority is true if the node is a block producer (overwrites previous settings)
	DefaultBabeAuthority = true
	// DefaultGrandpaAuthority is true if the node is a grandpa authority (overwrites previous settings)
	DefaultGrandpaAuthority = true

	// NetworkConfig

	// DefaultNetworkPort network port
	DefaultNetworkPort = uint32(7001)
	// DefaultNetworkBootnodes network bootnodes
	DefaultNetworkBootnodes = []string(nil)
	// DefaultNetworkProtocolID network protocol
	DefaultNetworkProtocolID = string("/gossamer/gssmr/0")
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
	DefaultRPCModules = []string{"system", "author", "chain", "state"}
	// DefaultRPCWSPort rpc websocket port
	DefaultRPCWSPort = uint32(8546)
)
