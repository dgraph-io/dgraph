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

	"github.com/ChainSafe/gossamer/internal/services"
	log "github.com/ChainSafe/log15"
	"github.com/libp2p/go-libp2p-core/network"
)

var _ services.Service = &Service{}

// Service describes a p2p service
type Service struct {
	ctx       context.Context
	host      *host
	status    *status
	discovery *discovery
	gossip    *gossip
	msgRec    <-chan Message
	msgSend   chan<- Message
}

// NewService creates a new p2p service from the configuration and message channels
func NewService(conf *Config, msgSend chan<- Message, msgRec <-chan Message) (*Service, error) {
	ctx := context.Background()

	// create a new host wrapper
	host, err := newHost(ctx, conf)
	if err != nil {
		return nil, err
	}

	// create a new status instance
	status, err := newStatus(host)
	if err != nil {
		return nil, err
	}

	// create a new discovery instance
	discovery, err := newDiscovery(ctx, host)
	if err != nil {
		return nil, err
	}

	// create a new gossip instance
	gossip, err := newGossip(host)
	if err != nil {
		return nil, err
	}

	p2p := &Service{
		ctx:       ctx,
		host:      host,
		status:    status,
		discovery: discovery,
		gossip:    gossip,
		msgRec:    msgRec,
		msgSend:   msgSend,
	}

	return p2p, err
}

// Start starts the p2p service
func (s *Service) Start() error {

	// receive messages from core service (including host status messages)
	go s.receiveCoreMessages()

	// TODO: ensure core service generates host status message and sends to p2p
	// service before connecting and exchanging status messages with peers

	// set connection and stream handler
	s.host.registerConnHandler(s.handleConn)
	s.host.registerStreamHandler(s.handleStream)
	s.host.printHostAddresses()

	s.host.bootstrap()

	// TODO: ensure bootstrap has connected to bootnodes and addresses have been
	// registered by the host before discovery attempts to connect to bootnodes

	// check if mDNS is enabled
	if !s.host.noMdns {

		// start mDNS discovery service
		s.discovery.startMdns()
	}

	return nil
}

// Stop shuts down the host and message channel to core service
func (s *Service) Stop() error {

	// close host and host services
	err := s.host.close()
	if err != nil {
		log.Error("Failed to close host", "err", err)
	}

	// close discovery and discovery services
	err = s.discovery.close()
	if err != nil {
		log.Error("Failed to close discovery", "err", err)
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

			// check if status exchange is enabled
			if !s.host.noStatus {

				// update host status message
				s.status.setHostMessage(msg)
			}
		}
	}
}

// handleConn starts processes that manage the connection
func (s *Service) handleConn(conn network.Conn) {

	// check if status exchange is enabled
	if !s.host.noStatus {

		// manage status messages for new connection
		s.status.handleConn(conn)
	}
}

// handleStream parses the message written to the data stream and calls the
// associated message handler (non-status or status) based on message type
func (s *Service) handleStream(stream network.Stream) {

	// parse message and exit on error
	msg, err := parseMessage(stream)
	if err != nil {
		log.Error("Failed to parse message from peer", "err", err)
		return // exit
	}

	log.Trace(
		"Received message from peer",
		"host", stream.Conn().LocalPeer(),
		"peer", stream.Conn().RemotePeer(),
		"type", msg.GetType(),
	)

	if msg.GetType() != StatusMsgType {
		s.handleMessage(stream, msg)
	} else {
		// handle status message with status submodule
		s.status.handleMessage(stream, msg.(*StatusMessage))
	}
}

// handleMessage handles non-status messages written to the stream
func (s *Service) handleMessage(stream network.Stream, msg Message) {
	peer := stream.Conn().RemotePeer()

	// check if status is disabled or peer status is confirmed
	if s.host.noStatus || s.status.confirmed(peer) {

		// send all non-status messages to core service
		s.msgSend <- msg
	}

	// check if gossip is enabled
	if !s.host.noGossip {

		// broadcast all non-status messages that have not been seen
		s.gossip.handleMessage(stream, msg)
	}
}

// Health returns information about host needed for the rpc server
func (s *Service) Health() Health {
	return Health{
		Peers:           s.host.peerCount(),
		IsSyncing:       false, // TODO
		ShouldHavePeers: !s.host.noBootstrap,
	}
}

// NetworkState returns information about host needed for the rpc server
func (s *Service) NetworkState() NetworkState {
	return NetworkState{
		PeerId: s.host.id(),
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
