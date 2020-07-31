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
	"context"
	"fmt"

	log "github.com/ChainSafe/log15"
	ds "github.com/ipfs/go-datastore"
	dsync "github.com/ipfs/go-datastore/sync"
	"github.com/libp2p/go-libp2p"
	libp2phost "github.com/libp2p/go-libp2p-core/host"
	libp2pnetwork "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p-core/protocol"
	kaddht "github.com/libp2p/go-libp2p-kad-dht"
	noise "github.com/libp2p/go-libp2p-noise"
	rhost "github.com/libp2p/go-libp2p/p2p/host/routed"
	ma "github.com/multiformats/go-multiaddr"
)

var defaultMaxPeerCount = 5

// host wraps libp2p host with network host configuration and services
type host struct {
	logger     log.Logger
	ctx        context.Context
	h          libp2phost.Host
	dht        *kaddht.IpfsDHT
	bootnodes  []peer.AddrInfo
	protocolID protocol.ID
}

// newHost creates a host wrapper with a new libp2p host instance
func newHost(ctx context.Context, cfg *Config, logger log.Logger) (*host, error) {

	// use "p2p" for multiaddress format
	ma.SwapToP2pMultiaddrs()

	// create multiaddress (without p2p identity)
	addr, err := ma.NewMultiaddr(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", cfg.Port))
	if err != nil {
		return nil, err
	}

	// create connection manager
	cm := newConnManager(defaultMaxPeerCount)

	// set libp2p host options
	opts := []libp2p.Option{
		libp2p.ListenAddrs(addr),
		libp2p.DisableRelay(),
		libp2p.Identity(cfg.privateKey),
		libp2p.NATPortMap(),
		libp2p.ConnectionManager(cm),
		libp2p.ChainOptions(libp2p.DefaultSecurity, libp2p.Security(noise.ID, noise.New)),
	}

	// create libp2p host instance
	h, err := libp2p.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	// create DHT service
	dht := kaddht.NewDHT(ctx, h, dsync.MutexWrap(ds.NewMapDatastore()))

	// wrap host and DHT service with routed host
	h = rhost.Wrap(h, dht)

	// format bootnodes
	bns, err := stringsToAddrInfos(cfg.Bootnodes)
	if err != nil {
		return nil, err
	}

	// format protocol id
	pid := protocol.ID(cfg.ProtocolID)

	logger = logger.New("module", "host")

	return &host{
		logger:     logger,
		ctx:        ctx,
		h:          h,
		dht:        dht,
		bootnodes:  bns,
		protocolID: pid,
	}, nil

}

// close closes host services and the libp2p host (host services first)
func (h *host) close() error {

	// close DHT service
	err := h.dht.Close()
	if err != nil {
		h.logger.Error("Failed to close DHT service", "error", err)
		return err
	}

	// close libp2p host
	err = h.h.Close()
	if err != nil {
		h.logger.Error("Failed to close libp2p host", "error", err)
		return err
	}

	return nil
}

// registerConnHandler registers the connection handler (see handleConn)
func (h *host) registerConnHandler(handler func(libp2pnetwork.Conn)) {
	h.h.Network().SetConnHandler(handler)
}

// registerStreamHandler registers the stream handler, appending the given sub-protocol to the main protocol ID
func (h *host) registerStreamHandler(sub protocol.ID, handler func(libp2pnetwork.Stream)) {
	h.h.SetStreamHandler(h.protocolID+sub, handler)
}

// connect connects the host to a specific peer address
func (h *host) connect(p peer.AddrInfo) (err error) {
	h.h.Peerstore().AddAddrs(p.ID, p.Addrs, peerstore.PermanentAddrTTL)
	err = h.h.Connect(h.ctx, p)
	return err
}

// bootstrap connects the host to the configured bootnodes
func (h *host) bootstrap() {
	for _, addrInfo := range h.bootnodes {
		err := h.connect(addrInfo)
		if err != nil {
			h.logger.Error("Failed to bootstrap peer", "error", err)
		}
	}
}

// send writes the given message to the outbound message stream for the given
// peer (gets the already opened outbound message stream or opens a new one).
func (h *host) send(p peer.ID, sub protocol.ID, msg Message) (err error) {
	encMsg, err := msg.Encode()
	if err != nil {
		return err
	}

	err = h.sendBytes(p, sub, encMsg)
	if err != nil {
		return err
	}

	h.logger.Trace(
		"Sent message to peer",
		"host", h.id(),
		"peer", p,
		"type", msg.Type(),
	)

	return nil
}

func (h *host) sendBytes(p peer.ID, sub protocol.ID, msg []byte) (err error) {
	// get outbound stream for given peer
	s := h.getStream(p, sub)

	// check if stream needs to be opened
	if s == nil {

		// open outbound stream with host protocol id
		s, err = h.h.NewStream(h.ctx, p, h.protocolID+sub)
		if err != nil {
			return err
		}

		h.logger.Trace(
			"Opened stream",
			"host", h.id(),
			"peer", p,
			"protocol", s.Protocol(),
		)
	}

	msgLen := uint64(len(msg))
	lenBytes := uint64ToLEB128(msgLen)
	msg = append(lenBytes, msg...)

	_, err = s.Write(msg)
	return err
}

// broadcast sends a message to each connected peer
func (h *host) broadcast(msg Message) {
	for _, p := range h.peers() {
		err := h.send(p, "", msg)
		if err != nil {
			h.logger.Error("Failed to broadcast message to peer", "peer", p, "error", err)
		}
	}
}

// broadcastExcluding sends a message to each connected peer except specified peer
func (h *host) broadcastExcluding(msg Message, peer peer.ID) {
	for _, p := range h.peers() {
		if p != peer {
			err := h.send(p, "", msg)
			if err != nil {
				h.logger.Error("Failed to send message during broadcast", "peer", p, "err", err)
			}
		}
	}
}

// getStream returns the outbound message stream for the given peer or returns
// nil if no outbound message stream exists. For each peer, each host opens an
// outbound message stream and writes to the same stream until closed or reset.
func (h *host) getStream(p peer.ID, sub protocol.ID) (stream libp2pnetwork.Stream) {
	conns := h.h.Network().ConnsToPeer(p)

	// loop through connections (only one for now)
	for _, conn := range conns {
		streams := conn.GetStreams()

		// loop through connection streams (unassigned streams and ipfs dht streams included)
		for _, stream := range streams {

			// return stream with matching host protocol id and stream direction outbound
			if stream.Protocol() == h.protocolID+sub && stream.Stat().Direction == libp2pnetwork.DirOutbound {
				return stream
			}
		}
	}
	return nil
}

// closePeer closes the peer connection
func (h *host) closePeer(peer peer.ID) error {
	err := h.h.Network().ClosePeer(peer)
	return err
}

// id returns the host id
func (h *host) id() peer.ID {
	return h.h.ID()
}

// Peers returns connected peers
func (h *host) peers() []peer.ID {
	return h.h.Network().Peers()
}

// peerConnected checks if peer is connected
func (h *host) peerConnected(peer peer.ID) bool {
	for _, p := range h.peers() {
		if p == peer {
			return true
		}
	}
	return false
}

// peerCount returns the number of connected peers
func (h *host) peerCount() int {
	peers := h.h.Network().Peers()
	return len(peers)
}

// addrInfos returns the libp2p AddrInfos of the host
func (h *host) addrInfos() (addrInfos []*peer.AddrInfo, err error) {
	for _, multiaddr := range h.multiaddrs() {
		addrInfo, err := peer.AddrInfoFromP2pAddr(multiaddr)
		if err != nil {
			return nil, err
		}
		addrInfos = append(addrInfos, addrInfo)
	}
	return addrInfos, nil
}

// multiaddrs returns the multiaddresses of the host
func (h *host) multiaddrs() (multiaddrs []ma.Multiaddr) {
	addrs := h.h.Addrs()
	for _, addr := range addrs {
		multiaddr, err := ma.NewMultiaddr(fmt.Sprintf("%s/p2p/%s", addr, h.id()))
		if err != nil {
			continue
		}
		multiaddrs = append(multiaddrs, multiaddr)
	}
	return multiaddrs
}
