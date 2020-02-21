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
	"strconv"
	"strings"

	log "github.com/ChainSafe/log15"
	"github.com/libp2p/go-libp2p-core/crypto"
)

// DefaultKeyFile KeyFile key
const DefaultKeyFile = "node.key"

// DefaultDataDir for the gossamer database and related blockchain files
const DefaultDataDir = "~/.gossamer"

// DefaultPort is the default por const
const DefaultPort = uint32(7000)

// DefaultRandSeed is a random key
const DefaultRandSeed = int64(0)

// DefaultProtocolID ID
const DefaultProtocolID = "/gossamer/dot/0"

// DefaultRoles full node
const DefaultRoles = byte(1)

// DefaultBootnodes holds the default bootnodes the client will always have
var DefaultBootnodes = []string(nil)

// Config is used to configure a network service
type Config struct {
	// BlockState interface
	BlockState BlockState
	// NetworkState interface
	NetworkState NetworkState
	// StorageState interface
	StorageState StorageState
	// Global data directory
	DataDir string
	// Role is a bitmap value whose bits represent difierent roles for the sender node (see Table E.2)
	Roles byte
	// Listening port
	Port uint32
	// If 0, random host ID will be generated; If non-0, deterministic ID will be produced, keys will not be loaded from data dir
	RandSeed int64
	// Peers used for bootstrapping
	Bootnodes []string
	// Protocol ID for network messages
	ProtocolID string
	// ProtocolVersion is the current protocol version, the last digit in the ProtocolID
	ProtocolVersion uint32
	// MinSupportedVersion is the minimum supported protocol version (defaults to current ProtocolVersion)
	MinSupportedVersion uint32
	// Disables bootstrapping
	NoBootstrap bool
	// Disables MDNS discovery
	NoMdns bool
	// Identity key for node
	privateKey crypto.PrivKey
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
		log.Warn("Bootstrap is enabled and no bootstrap nodes are defined")
	}

	return nil
}

func (c *Config) checkState() (err error) {
	if c.BlockState == nil {
		err = errors.New("Failed to build configuration: BlockState required")
	}
	if c.NetworkState == nil {
		err = errors.New("Failed to build configuration: NetworkState required")
	}
	if c.StorageState == nil {
		err = errors.New("Failed to build configuration: StorageState required")
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
			log.Trace(
				"Generating new p2p identity",
				"DataDir", c.DataDir,
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
		log.Warn(
			"Generating p2p identity from seed",
			"DataDir", c.DataDir,
			"RandSeed", c.RandSeed,
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
			"ProtocolID not defined, using DefaultProtocolID",
			"DefaultProtocolID", DefaultProtocolID,
		)
		c.ProtocolID = DefaultProtocolID
	}

	if c.ProtocolVersion == 0 {
		s := strings.Split(c.ProtocolID, "/")
		// expecting default protocol format ("/gossamer/dot/0")
		if len(s) != 4 {
			log.Warn(
				"Unable to parse ProtocolID, using DefaultProtocolVersion",
				"DefaultProtocolVersion", 0,
			)
		} else {
			// get the last item in the slice ("0" in default protocol format)
			i, err := strconv.Atoi(s[len(s)-1])
			if err != nil {
				log.Warn(
					"Unable to parse ProtocolID, using DefaultProtocolVersion",
					"DefaultProtocolVersion", 0,
				)
			} else {
				c.ProtocolVersion = uint32(i)
			}
		}
	}

	if c.MinSupportedVersion < c.ProtocolVersion {
		log.Warn("MinSupportedVersion less than ProtocolVersion, using ProtocolVersion",
			"ProtocolVersion", c.ProtocolVersion,
		)
		c.MinSupportedVersion = c.ProtocolVersion
	}

	return nil
}
