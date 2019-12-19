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

package p2p

import (
	log "github.com/ChainSafe/log15"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
)

const KeyFile = "node.key"

const DefaultProtocolId = "/gossamer/dot/0"

// Config is used to configure a p2p service
type Config struct {
	// Peers used for bootstrapping
	BootstrapNodes []string
	// Protocol ID for network messages
	ProtocolId string
	// Listening port
	Port uint32
	// If 0, random host ID will be generated; If non-0, deterministic ID will be produced, keys will not be loaded from data dir
	RandSeed int64
	// Disables bootstrapping
	NoBootstrap bool
	// Disables MDNS discovery
	NoMdns bool
	// Global data directory
	DataDir string
	// Identity key for node
	privateKey crypto.PrivKey
}

// bootnodes formats the configuration bootnodes
func (c *Config) bootnodes() (peer []peer.AddrInfo, err error) {
	bootnodes, err := stringsToAddrInfos(c.BootstrapNodes)
	if err != nil {
		return nil, err
	}
	return bootnodes, nil
}

// protocolId formats the configuration protocol id
func (c *Config) protocolId() protocol.ID {
	return protocol.ID(c.ProtocolId)
}

// build checks the configuration, sets up the private key for the p2p service,
// and applies default values where appropriate
func (c *Config) build() error {
	if c.ProtocolId == "" {
		c.ProtocolId = DefaultProtocolId
	}

	if !c.NoBootstrap && len(c.BootstrapNodes) == 0 {
		log.Warn("Bootstrap is enabled and no bootstrap nodes are defined")
	}

	// check if random seed set
	if c.RandSeed == 0 {

		// load existing key or create random key
		err := c.setupKey()
		if err != nil {
			return err
		}

	} else {
		log.Warn(
			"Generating temporary deterministic p2p identity",
			"directory", c.DataDir,
			"keyfile", KeyFile,
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

// setupKey attempts to load the p2p private key required to start the p2p
// servce, if a key does not exist or cannot be loaded, it creates a new key
// using the random seed (if random seed is not set, creates new random key)
func (c *Config) setupKey() error {

	// attempt to load existing key
	key, err := loadKey(c.DataDir)
	if err != nil {
		return err
	}

	// check if key set
	if key == nil {
		log.Trace(
			"Generating new p2p identity",
			"directory", c.DataDir,
			"keyfile", KeyFile,
		)

		// generate key
		key, err = generateKey(c.RandSeed, c.DataDir)
		if err != nil {
			return err
		}

	} else {

		// get p2p identity from private key
		id, err := peer.IDFromPrivateKey(key)
		if err != nil {
			return err
		}

		log.Trace(
			"Using existing p2p identity",
			"directory", c.DataDir,
			"keyfile", KeyFile,
			"id", id,
		)
	}

	// set private key
	c.privateKey = key

	return nil
}
