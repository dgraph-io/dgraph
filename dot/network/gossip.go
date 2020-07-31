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
// GNU Lesser General Public License for more detailg.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the gossamer library. If not, see <http://www.gnu.org/licenses/>.

package network

import (
	"sync"

	log "github.com/ChainSafe/log15"
	"github.com/libp2p/go-libp2p-core/peer"
)

// gossip submodule
type gossip struct {
	logger  log.Logger
	host    *host
	hasSeen *sync.Map
}

// newGossip creates a new gossip instance from the host
func newGossip(host *host) *gossip {
	return &gossip{
		logger:  host.logger.New("module", "gossip"),
		host:    host,
		hasSeen: &sync.Map{},
	}
}

// handleMessage broadcasts messages that have not been seen
func (g *gossip) handleMessage(msg Message, from peer.ID) {

	// check if message has not been seen
	if seen, ok := g.hasSeen.Load(msg.IDString()); !ok || !seen.(bool) {

		// set message to has been seen
		g.hasSeen.Store(msg.IDString(), true)

		g.logger.Trace(
			"Gossiping message from peer",
			"host", g.host.id(),
			"type", msg.Type(),
		)

		// broadcast message to connected peers
		g.host.broadcastExcluding(msg, from)
	}
}
