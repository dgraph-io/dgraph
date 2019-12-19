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
	libp2pdiscovery "github.com/libp2p/go-libp2p/p2p/discovery"
)

const mdnsPeriod = time.Minute

// See https://godoc.org/github.com/libp2p/go-libp2p/p2p/discovery#Notifee
type Notifee struct {
	ctx  context.Context
	host *host
}

// discovery submodule
type discovery struct {
	ctx  context.Context
	host *host
	mdns libp2pdiscovery.Service
}

// newDiscovery creates a new discovery instance from the host
func newDiscovery(ctx context.Context, host *host) (d *discovery, err error) {
	d = &discovery{
		ctx:  ctx,
		host: host,
	}
	return d, err
}

// close shuts down any running discovery services
func (d *discovery) close() error {

	// check if mdns service is running
	if d.mdns != nil {

		// close mdns service
		err := d.mdns.Close()
		if err != nil {
			log.Error("Failed to close mDNS discovery service", "err", err)
		}
	}

	return nil
}

// startMdns starts a new mDNS discovery service
func (d *discovery) startMdns() {
	log.Trace(
		"Starting mDNS discovery service...",
		"host", d.host.id(),
		"period", mdnsPeriod,
		"protocol", d.host.protocolId,
	)

	// create and start mDNS discovery service
	mdns, err := libp2pdiscovery.NewMdnsService(
		d.ctx,
		d.host.h,
		mdnsPeriod,
		string(d.host.protocolId),
	)
	if err != nil {
		log.Error("Failed to start mDNS discovery service", "err", err)
	}

	// register Notifee on mDNS discovery service
	mdns.RegisterNotifee(Notifee{
		ctx:  d.ctx,
		host: d.host,
	})

	d.mdns = mdns
}

// HandlePeerFound is event handler called when a peer is found with discovery
func (n Notifee) HandlePeerFound(p peer.AddrInfo) {
	log.Trace(
		"Peer found using mDNS discovery service",
		"host", n.host.id(),
		"peer", p.ID,
	)

	// connect to found peer
	err := n.host.connect(p)
	if err != nil {
		log.Error("Failed to connect to peer using mDNS discovery service", "err", err)
	}
}
