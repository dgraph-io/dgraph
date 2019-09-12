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
	"context"

	log "github.com/ChainSafe/log15"
	host "github.com/libp2p/go-libp2p-core/host"
	peer "github.com/libp2p/go-libp2p-core/peer"
	ps "github.com/libp2p/go-libp2p-core/peerstore"
)

// See https://godoc.org/github.com/libp2p/go-libp2p/p2p/discovery#Notifee
type Notifee struct {
	ctx  context.Context
	host host.Host
}

// HandlePeerFound is invoked when a peer in discovered via mDNS
func (n Notifee) HandlePeerFound(p peer.AddrInfo) {
	log.Info("mdns", "peer found", p)

	n.host.Peerstore().AddAddrs(p.ID, p.Addrs, ps.PermanentAddrTTL)
	err := n.host.Connect(n.ctx, p)
	if err != nil {
		log.Error("mdns", "connect error", err)
	}
}
