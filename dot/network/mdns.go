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
	"context"
	"time"

	log "github.com/ChainSafe/log15"
	"github.com/libp2p/go-libp2p-core/peer"
	discovery "github.com/libp2p/go-libp2p/p2p/discovery"
)

// MDNSPeriod is 1 minute
const MDNSPeriod = time.Minute

// Notifee See https://godoc.org/github.com/libp2p/go-libp2p/p2p/discovery#Notifee
type Notifee struct {
	logger log.Logger
	ctx    context.Context
	host   *host
}

// mdns submodule
type mdns struct {
	logger log.Logger
	host   *host
	mdns   discovery.Service
}

// newMDNS creates a new mDNS instance from the host
func newMDNS(host *host) *mdns {
	return &mdns{
		logger: host.logger.New("module", "mdns"),
		host:   host,
	}
}

// startMDNS starts a new mDNS discovery service
func (m *mdns) start() {
	m.logger.Trace(
		"Starting mDNS discovery service...",
		"host", m.host.id(),
		"period", MDNSPeriod,
		"protocol", m.host.protocolID,
	)

	// create and start service
	mdns, err := discovery.NewMdnsService(
		m.host.ctx,
		m.host.h,
		MDNSPeriod,
		string(m.host.protocolID),
	)
	if err != nil {
		m.logger.Error("Failed to start mDNS discovery service", "error", err)
		return
	}

	// register Notifee on service
	mdns.RegisterNotifee(Notifee{
		logger: m.logger,
		ctx:    m.host.ctx,
		host:   m.host,
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
			m.logger.Error("Failed to close mDNS discovery service", "error", err)
			return err
		}
	}

	return nil
}

// HandlePeerFound is event handler called when a peer is found
func (n Notifee) HandlePeerFound(p peer.AddrInfo) {
	n.logger.Trace(
		"Peer found using mDNS discovery",
		"host", n.host.id(),
		"peer", p.ID,
	)

	// connect to found peer
	err := n.host.connect(p)
	if err != nil {
		n.logger.Error("Failed to connect to peer using mDNS discovery", "error", err)
	}
}
