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
	"context"
	"time"

	log "github.com/ChainSafe/log15"
	"github.com/libp2p/go-libp2p-core/peer"
	discovery "github.com/libp2p/go-libp2p/p2p/discovery"
)

const MdnsPeriod = time.Minute

// See https://godoc.org/github.com/libp2p/go-libp2p/p2p/discovery#Notifee
type Notifee struct {
	ctx  context.Context
	host *host
}

// mdns submodule
type mdns struct {
	host *host
	mdns discovery.Service
}

// newMdns creates a new mDNS instance from the host
func newMdns(host *host) *mdns {
	return &mdns{
		host: host,
	}
}

// startMdns starts a new mDNS discovery service
func (m *mdns) start() {
	log.Trace(
		"Starting mDNS discovery service...",
		"host", m.host.id(),
		"period", MdnsPeriod,
		"protocol", m.host.protocolId,
	)

	// create and start service
	mdns, err := discovery.NewMdnsService(
		m.host.ctx,
		m.host.h,
		MdnsPeriod,
		string(m.host.protocolId),
	)
	if err != nil {
		log.Error("Failed to start mDNS discovery service", "err", err)
	}

	// register Notifee on service
	mdns.RegisterNotifee(Notifee{
		ctx:  m.host.ctx,
		host: m.host,
	})

	m.mdns = mdns
}

// close shuts down the mDNS discovery service
func (m *mdns) close() error {

	// check if service is running
	if m.mdns != nil {

		// close service
		err := m.mdns.Close()
		if err != nil {
			log.Error("Failed to close mDNS discovery service", "err", err)
			return err
		}
	}

	return nil
}

// HandlePeerFound is event handler called when a peer is found
func (n Notifee) HandlePeerFound(p peer.AddrInfo) {
	log.Trace(
		"Peer found using mDNS discovery",
		"host", n.host.id(),
		"peer", p.ID,
	)

	// connect to found peer
	err := n.host.connect(p)
	if err != nil {
		log.Error("Failed to connect to peer using mDNS discovery", "err", err)
	}
}
