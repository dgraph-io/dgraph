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

package p2p

import (
	log "github.com/ChainSafe/log15"
	"github.com/libp2p/go-libp2p-core/network"
)

// gossip submodule
type gossip struct {
	host    *host
	hasSeen map[string]bool
}

// newGossip creates a new gossip instance from the host
func newGossip(host *host) (g *gossip, err error) {
	g = &gossip{
		host:    host,
		hasSeen: make(map[string]bool),
	}
	return g, err
}

// handleMessage broadcasts messages that have not been seen
func (g *gossip) handleMessage(stream network.Stream, msg Message) {

	// check if message has not been seen
	if !g.hasSeen[msg.Id()] {

		// set message to has been seen
		g.hasSeen[msg.Id()] = true

		log.Trace(
			"Gossiping message from peer",
			"host", g.host.id(),
			"type", msg.GetType(),
		)

		// broadcast message to connected peers
		g.host.broadcast(msg)
	}
}
