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

	"github.com/ChainSafe/gossamer/common"
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

// NetworkState is network information about host needed for the rpc server
type NetworkState struct {
	PeerId string
}

// PeerInfo is network information about peers needed for the rpc server
type PeerInfo struct {
	PeerId          string
	Roles           byte
	ProtocolVersion uint32
	BestHash        common.Hash
	BestNumber      uint64
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

	// set connection and stream handler
	s.host.registerConnHandler(s.handleConn)
	s.host.registerStreamHandler(s.handleStream)

	s.host.bootstrap()
	s.host.printHostAddresses()

	// check if mDNS discovery service is enabled
	if !s.host.noMdns {

		// start mDNS discovery service
		s.discovery.startMdns()
	}

	// receive messages from core service
	go s.receiveCoreMessages()

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

		log.Trace(
			"Broadcasting message from core service",
			"host", s.host.id(),
			"type", msg.GetType(),
		)

		// broadcast message to connected peers
		s.host.broadcast(msg)
	}
}

// handleConn starts processes that manage the connection
func (s *Service) handleConn(conn network.Conn) {

	// check if status exchange is enabled
	if !s.host.noStatus {

		// starts sending status messages to connected peer
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
		// handle non-status message with service
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
	if s.host.noStatus || s.status.peerConfirmed[peer] {

		// send all non-status messages to core service
		s.msgSend <- msg
	}

	// check if gossip is enabled
	if !s.host.noGossip {

		// broadcast message if message has not been seen
		s.gossip.handleMessage(stream, msg)
	}
}

// ID returns host id
func (s *Service) ID() string {
	return s.host.id()
}

// NetworkState returns information about host needed for the rpc server
func (s *Service) NetworkState() (ns NetworkState) {
	return NetworkState{
		PeerId: s.host.id(),
	}
}

// NoBootstrapping returns true if bootstrapping is disabled, otherwise false
func (s *Service) NoBootstrapping() bool {
	return s.host.noBootstrap
}

// Peers returns connected peers
func (s *Service) Peers() []string {
	return peerIdsToStrings(s.host.peers())
}

// PeerCount returns the number of connected peers
func (s *Service) PeerCount() int {
	return s.host.peerCount()
}

// PeerInfo returns information about a peer needed for the rpc server
func (s *Service) PeerInfo(peerId string) (peerInfo PeerInfo) {
	p := stringToPeerId(peerId)
	if s.status.peerConfirmed[p] {
		peerInfo = s.status.peerInfo[p]
	}
	return peerInfo
}
