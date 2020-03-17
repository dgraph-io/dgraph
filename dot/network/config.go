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

package network

import (
	"errors"
	"math/big"
	"path"
	"strconv"
	"strings"

	log "github.com/ChainSafe/log15"
	"github.com/libp2p/go-libp2p-core/crypto"
)

// DefaultKeyFile the default value for KeyFile
const DefaultKeyFile = "node.key"

// DefaultDataDir the default value for Config.DataDir
const DefaultDataDir = "~/.gossamer/gssmr"

// DefaultPort the default value for Config.Port
const DefaultPort = uint32(7000)

// DefaultRandSeed the default value for Config.RandSeed (0 = non-deterministic)
const DefaultRandSeed = int64(0)

// DefaultProtocolID the default value for Config.ProtocolID
const DefaultProtocolID = "/gossamer/gssmr/0"

// DefaultProtocolVersion the default value for Config.ProtocolVersion
const DefaultProtocolVersion = 0

// DefaultRoles the default value for Config.Roles (0 = no network, 1 = full node)
const DefaultRoles = byte(1)

// DefaultBootnodes the default value for Config.Bootnodes
var DefaultBootnodes = []string(nil)

// Config is used to configure a network service
type Config struct {
	// BlockState the block state's interface
	BlockState BlockState
	// NetworkState the network state's interface
	NetworkState NetworkState
	// DataDir the data directory for the node
	DataDir string
	// Roles a bitmap value that represents the different roles for the sender node (see Table E.2)
	Roles byte
	// Port the network port used for listening
	Port uint32
	// RandSeed the seed used to generate the network p2p identity (0 = non-deterministic random seed)
	RandSeed int64
	// Bootnodes the peer addresses used for bootstrapping
	Bootnodes []string
	// ProtocolID the protocol ID for network messages
	ProtocolID string
	// ProtocolVersion the protocol version for network messages (the third item in the ProtocolID)
	ProtocolVersion uint32
	// MinSupportedVersion the minimum supported protocol version (defaults to current ProtocolVersion)
	MinSupportedVersion uint32
	// NoBootstrap disables bootstrapping
	NoBootstrap bool
	// NoMDNS disables MDNS discovery
	NoMDNS bool
	// NoStatus disables the status message exchange protocol
	NoStatus bool
	// privateKey the private key for the network p2p identity
	privateKey crypto.PrivKey
	// SyncChan is the channel for syncing
	SyncChan chan<- *big.Int
}

// build checks the configuration, sets up the private key for the network service,
// and applies default values where appropriate
func (c *Config) build() error {

	// check state configuration
	err := c.checkState()
	if err != nil {
		return err
	}

	if c.DataDir == "" {
		c.DataDir = DefaultDataDir
	}

	if c.Roles == 0 {
		c.Roles = DefaultRoles
	}

	if c.Port == 0 {
		c.Port = DefaultPort
	}

	// build identity configuration
	err = c.buildIdentity()
	if err != nil {
		return err
	}

	// build protocol configuration
	err = c.buildProtocol()
	if err != nil {
		return err
	}

	// check bootnoode configuration
	if !c.NoBootstrap && len(c.Bootnodes) == 0 {
		log.Warn("[network] Bootstrap is enabled but no bootstrap nodes are defined")
	}

	return nil
}

func (c *Config) checkState() (err error) {
	// set NoStatus to true if we don't need BlockState
	if c.BlockState == nil && !c.NoStatus {
		err = errors.New("failed to build configuration: BlockState required")
	}

	if c.NetworkState == nil {
		err = errors.New("failed to build configuration: NetworkState required")
	}

	return err
}

// buildIdentity attempts to load the private key required to start the network
// service, if a key does not exist or cannot be loaded, it creates a new key
// using the random seed (if random seed is not set, creates new random key)
func (c *Config) buildIdentity() error {
	if c.RandSeed == 0 {

		// attempt to load existing key
		key, err := loadKey(c.DataDir)
		if err != nil {
			return err
		}

		// generate key if no key exists
		if key == nil {
			log.Info(
				"[network] Generating p2p identity",
				"RandSeed", c.RandSeed,
				"KeyFile", path.Join(c.DataDir, DefaultKeyFile),
			)

			// generate key
			key, err = generateKey(c.RandSeed, c.DataDir)
			if err != nil {
				return err
			}
		}

		// set private key
		c.privateKey = key
	} else {
		log.Info(
			"[network] Generating p2p identity from seed",
			"RandSeed", c.RandSeed,
			"KeyFile", path.Join(c.DataDir, DefaultKeyFile),
		)

		// generate temporary deterministic key
		key, err := generateKey(c.RandSeed, c.DataDir)
		if err != nil {
			return err
		}

		// set private key
		c.privateKey = key
	}

	return nil
}

// buildProtocol verifies and applies defaults to the protocol configuration
func (c *Config) buildProtocol() error {
	if c.ProtocolID == "" {
		log.Warn(
			"[network] ProtocolID not defined, using DefaultProtocolID",
			"DefaultProtocolID", DefaultProtocolID,
		)
		c.ProtocolID = DefaultProtocolID
	}

	if c.ProtocolVersion == 0 {
		s := strings.Split(c.ProtocolID, "/")
		// expecting the default protocol format ("/gossamer/gssmr/0")
		if len(s) != 4 {
			log.Warn(
				"[network] Unable to parse ProtocolID, using DefaultProtocolVersion",
				"DefaultProtocolVersion", DefaultProtocolVersion,
			)
		} else {
			// get the last item in the slice ("0" in the default protocol format)
			i, err := strconv.Atoi(s[len(s)-1])
			if err != nil {
				log.Warn(
					"[network] Unable to parse ProtocolID, using DefaultProtocolVersion",
					"DefaultProtocolVersion", DefaultProtocolVersion,
				)
			} else {
				c.ProtocolVersion = uint32(i)
			}
		}
	}

	if c.MinSupportedVersion < c.ProtocolVersion {
		log.Warn(
			"[network] MinSupportedVersion less than ProtocolVersion, using ProtocolVersion",
			"ProtocolVersion", c.ProtocolVersion,
		)
		c.MinSupportedVersion = c.ProtocolVersion
	}

	return nil
}
