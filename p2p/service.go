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
	"bufio"
	"context"

	"github.com/ChainSafe/gossamer/internal/services"
	log "github.com/ChainSafe/log15"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
)

var _ services.Service = &Service{}

// Service describes a p2p service
type Service struct {
	ctx         context.Context
	host        *host
	mdns        *mdns
	status      *status
	gossip      *gossip
	msgRec      <-chan Message
	msgSend     chan<- Message
	noBootstrap bool
	noMdns      bool
	noStatus    bool // internal option
	noGossip    bool // internal option
}

// NewService creates a new p2p service from the configuration and message channels
func NewService(cfg *Config, msgSend chan<- Message, msgRec <-chan Message) (*Service, error) {
	ctx := context.Background()

	// build configuration
	err := cfg.build()
	if err != nil {
		return nil, err
	}

	// create a new host instance
	host, err := newHost(ctx, cfg)
	if err != nil {
		return nil, err
	}

	p2p := &Service{
		ctx:         ctx,
		host:        host,
		mdns:        newMdns(host),
		status:      newStatus(host),
		gossip:      newGossip(host),
		msgRec:      msgRec,
		msgSend:     msgSend,
		noBootstrap: cfg.NoBootstrap,
		noMdns:      cfg.NoMdns,
	}

	return p2p, err
}

// Start starts the p2p service
func (s *Service) Start() error {

	// receive messages from core service (including host status messages)
	go s.receiveCoreMessages()

	// TODO: ensure core service generates host status message and sends to p2p
	// service before connecting and exchanging status messages with peers

	s.host.registerConnHandler(s.handleConn)
	s.host.registerStreamHandler(s.handleStream)

	s.host.printHostAddresses()

	if !s.noBootstrap {
		s.host.bootstrap()
	}

	// TODO: ensure bootstrap has connected to bootnodes and addresses have been
	// registered by the host before mDNS attempts to connect to bootnodes

	if !s.noMdns {
		s.mdns.start()
	}

	return nil
}

// Stop closes running instances of the host and network services as well as
// the message channel from the p2p service to the core service (services that
// are dependent on the host instance should be closed first)
func (s *Service) Stop() error {

	// close mDNS discovery service
	err := s.mdns.close()
	if err != nil {
		log.Error("Failed to close mDNS discovery service", "err", err)
	}

	// close host and host services
	err = s.host.close()
	if err != nil {
		log.Error("Failed to close host", "err", err)
	}

	// close channel to core service
	if s.msgSend != nil {
		close(s.msgSend)
	}

	return nil
}

// receiveCoreMessages broadcasts messages from the core service
func (s *Service) receiveCoreMessages() {
	for {
		// receive message from core service
		msg := <-s.msgRec

		// check if non-status message
		if msg.GetType() != StatusMsgType {

			log.Trace(
				"Broadcasting message from core service",
				"host", s.host.id(),
				"type", msg.GetType(),
			)

			// broadcast message to connected peers
			s.host.broadcast(msg)

		} else {

			// check if status is enabled
			if !s.noStatus {

				// update host status message
				s.status.setHostMessage(msg)
			}
		}
	}
}

// handleConn starts processes that manage the connection
func (s *Service) handleConn(conn network.Conn) {

	// check if status is enabled
	if !s.noStatus {

		// manage status messages for new connection
		s.status.handleConn(conn)
	}
}

// handleStream starts reading from the inbound message stream (substream with
// a matching protocol id that was opened by the connected peer) and continues
// reading until the inbound message stream is closed or reset.
func (s *Service) handleStream(stream network.Stream) {
	peer := stream.Conn().RemotePeer()

	// create buffer stream for non-blocking read
	r := bufio.NewReader(stream)

	for {
		// decode message based on message type
		msg, err := decodeMessage(r)
		if err != nil {

			// exit loop if last received byte was nil (stream closed or reset)
			ub := r.UnreadByte()
			if ub == nil {
				return // exit
			}

			log.Error("Failed to decode message from peer", "peer", peer, "err", err)
			return // exit
		}

		// handle message based on peer status and message type
		s.handleMessage(peer, msg)
	}

	// the stream stays open until closed or reset
}

// handleMessage handles the message based on peer status and message type
func (s *Service) handleMessage(peer peer.ID, msg Message) {
	log.Trace(
		"Received message from peer",
		"host", s.host.id(),
		"peer", peer,
		"type", msg.GetType(),
	)

	if msg.GetType() != StatusMsgType {

		// check if status is disabled or peer status is confirmed
		if s.noStatus || s.status.confirmed(peer) {

			// send non-status message from confirmed peer to core service
			s.msgSend <- msg
		}

		// check if gossip is enabled
		if !s.noGossip {

			// handle non-status message from peer with gossip submodule
			s.gossip.handleMessage(msg)
		}

	} else {

		// check if status is enabled
		if !s.noStatus {

			// handle status message from peer with status submodule
			s.status.handleMessage(peer, msg.(*StatusMessage))
		}
	}
}

// Health returns information about host needed for the rpc server
func (s *Service) Health() Health {
	return Health{
		Peers:           s.host.peerCount(),
		IsSyncing:       false, // TODO
		ShouldHavePeers: !s.noBootstrap,
	}
}

// NetworkState returns information about host needed for the rpc server and the runtime
func (s *Service) NetworkState() NetworkState {
	return NetworkState{
		PeerId: s.host.id().String(),
	}
}

// Peers returns information about connected peers needed for the rpc server
func (s *Service) Peers() (peers []PeerInfo) {
	for _, p := range s.host.peers() {
		if s.status.confirmed(p) {
			msg := s.status.peerMessage[p]
			peers = append(peers, PeerInfo{
				PeerId:          p.String(),
				Roles:           msg.Roles,
				ProtocolVersion: msg.ProtocolVersion,
				BestHash:        msg.BestBlockHash,
				BestNumber:      msg.BestBlockNumber,
			})
		}
	}
	return peers
}
