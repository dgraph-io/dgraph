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
	"fmt"

	"github.com/ChainSafe/gossamer/common"
	log "github.com/ChainSafe/log15"
	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/sync"
	"github.com/libp2p/go-libp2p"
	libp2phost "github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p-core/protocol"
	kaddht "github.com/libp2p/go-libp2p-kad-dht"
	rhost "github.com/libp2p/go-libp2p/p2p/host/routed"
	ma "github.com/multiformats/go-multiaddr"
)

// host wraps libp2p host with host services and information
type host struct {
	ctx        context.Context
	h          libp2phost.Host
	dht        *kaddht.IpfsDHT
	bootnodes  []peer.AddrInfo
	protocolId protocol.ID
}

// newHost creates a host wrapper with a new libp2p host instance
func newHost(ctx context.Context, cfg *Config) (*host, error) {

	// use "p2p" for multiaddress format
	ma.SwapToP2pMultiaddrs()

	// create multiaddress (without p2p identity)
	addr, err := ma.NewMultiaddr(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", cfg.Port))
	if err != nil {
		return nil, err
	}

	// set connection manager
	cm := &ConnManager{}

	// set libp2p host options
	opts := []libp2p.Option{
		libp2p.ListenAddrs(addr),
		libp2p.DisableRelay(),
		libp2p.Identity(cfg.privateKey),
		libp2p.NATPortMap(),
		libp2p.Ping(true),
		libp2p.ConnectionManager(cm),
	}

	// create libp2p host instance
	h, err := libp2p.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	// create DHT service
	dht := kaddht.NewDHT(ctx, h, sync.MutexWrap(ds.NewMapDatastore()))

	// wrap host and DHT service with routed host
	h = rhost.Wrap(h, dht)

	// format bootnodes
	bns, err := cfg.bootnodes()
	if err != nil {
		return nil, err
	}

	// format protocol id
	pid := cfg.protocolId()

	return &host{
		ctx:        ctx,
		h:          h,
		dht:        dht,
		bootnodes:  bns,
		protocolId: pid,
	}, nil

}

// close closes host services and the libp2p host (host services first)
func (h *host) close() error {

	// close DHT service
	err := h.dht.Close()
	if err != nil {
		log.Error("Failed to close DHT service", "err", err)
		return err
	}

	// close libp2p host
	err = h.h.Close()
	if err != nil {
		log.Error("Failed to close libp2p host", "err", err)
		return err
	}

	return nil
}

// registerConnHandler registers the connection handler (see handleConn)
func (h *host) registerConnHandler(handler func(network.Conn)) {
	h.h.Network().SetConnHandler(handler)
}

// registerStreamHandler registers the stream handler (see handleStream)
func (h *host) registerStreamHandler(handler func(network.Stream)) {
	h.h.SetStreamHandler(h.protocolId, handler)
}

// printHostAddresses prints host multiaddresses to console
func (h *host) printHostAddresses() {
	fmt.Println("Listening on the following addresses...")
	for _, addr := range h.multiaddrs() {
		fmt.Println(addr)
	}
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
			log.Error("Failed to bootstrap peer", "err", err)
		}
	}
}

// ping pings a peer using DHT
func (h *host) ping(peer peer.ID) error {
	return h.dht.Ping(h.ctx, peer)
}

// send sends a non-status message to a specific peer
func (h *host) send(p peer.ID, msg Message) (err error) {

	// create new stream with host protocol id
	s, err := h.h.NewStream(h.ctx, p, h.protocolId)
	if err != nil {
		return err
	}

	log.Trace(
		"Opened stream",
		"host", h.id(),
		"peer", p,
		"protocol", s.Protocol(),
	)

	encMsg, err := msg.Encode()
	if err != nil {
		return err
	}

	_, err = s.Write(common.Uint16ToBytes(uint16(len(encMsg)))[0:1])
	if err != nil {
		return err
	}

	_, err = s.Write(encMsg)
	if err != nil {
		return err
	}

	log.Trace(
		"Sent message",
		"host", h.id(),
		"peer", p,
		"type", msg.GetType(),
	)

	return nil
}

// broadcast sends a message to each connected peer
func (h *host) broadcast(msg Message) {
	for _, p := range h.peers() {
		err := h.send(p, msg)
		if err != nil {
			log.Error("Failed to send message during broadcast", "peer", p, "err", err)
		}
	}
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
