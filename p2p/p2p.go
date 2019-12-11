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
	"time"

	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/internal/services"
	log "github.com/ChainSafe/log15"
	"github.com/libp2p/go-libp2p-core/network"
	net "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
)

var _ services.Service = &Service{}

// SendStatusInterval is the time between sending status messages
const SendStatusInterval = 5 * time.Minute

// Service describes a p2p service
type Service struct {
	ctx       context.Context
	host      *host
	discovery *discovery
	gossip    *gossip
	msgRec    <-chan Message
	msgSend   chan<- Message
}

// TODO: use generated status message
var statusMessage = &StatusMessage{
	ProtocolVersion:     0,
	MinSupportedVersion: 0,
	Roles:               0,
	BestBlockNumber:     0,
	BestBlockHash:       common.Hash{0x00},
	GenesisHash:         common.Hash{0x00},
	ChainStatus:         []byte{0},
}

// NewService creates a new p2p service from the configuration and message channels
func NewService(conf *Config, msgSend chan<- Message, msgRec <-chan Message) (*Service, error) {
	ctx := context.Background()

	host, err := newHost(ctx, conf)
	if err != nil {
		return nil, err
	}

	discovery, err := newDiscovery(ctx, host)
	if err != nil {
		return nil, err
	}

	gossip, err := newGossip(host)
	if err != nil {
		return nil, err
	}

	p2p := &Service{
		ctx:       ctx,
		host:      host,
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

// handleConn starts processes that manage the connection
func (s *Service) handleConn(conn network.Conn) {

	// check if status exchange is enabled
	if !s.host.noStatus {

		// starts sending status messages to connected peer
		go s.sendStatusMessages(conn.RemotePeer())

	}
}

// sendStatusMessages starts sending status messages to the connected peer
func (s *Service) sendStatusMessages(peer peer.ID) {
	for {
		// TODO: use generated status message
		msg := statusMessage

		// send status message to connected peer
		err := s.host.send(peer, msg)
		if err != nil {
			log.Error("Failed to send status message", "err", err)
		}

		// wait between sending messages
		time.Sleep(SendStatusInterval)
	}
}

// receiveCoreMessages broadcasts messages from the core service
func (s *Service) receiveCoreMessages() {
	for {
		// receive message from core service
		msg := <-s.msgRec

		log.Trace(
			"Broadcasting message from core service",
			"host", s.ID(),
			"type", msg.GetType(),
		)

		// broadcast message to connected peers
		s.host.broadcast(msg)
	}
}

// handleStream parses the message written to the data stream and calls the
// associated message handler (status or non-status) based on message type
func (s *Service) handleStream(stream net.Stream) {

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
		s.handleStatusMessage(stream, msg)
	}

}

// handleMessage handles non-status messages written to the stream
func (s *Service) handleMessage(stream network.Stream, msg Message) {

	// check if status exchange is disabled or peer status is confirmed
	if s.host.noStatus || s.host.peerStatus[stream.Conn().RemotePeer()] {

		// send all non-status messages to core service
		s.msgSend <- msg

	}

	// check if gossip is enabled
	if !s.host.noGossip {

		// broadcast message if message has not been seen
		s.gossip.handleMessage(stream, msg)

	}
}

// handleStatusMessage handles status messages written to the stream
func (s *Service) handleStatusMessage(stream network.Stream, msg Message) {

	// TODO: use generated status message
	hostStatus := statusMessage

	switch {

	// TODO: implement status message validation
	case hostStatus.String() == msg.String():
		log.Trace(
			"Received valid status message",
			"host", stream.Conn().LocalPeer(),
			"peer", stream.Conn().RemotePeer(),
		)

		s.host.peerStatus[stream.Conn().RemotePeer()] = true

	default:
		log.Trace(
			"Received invalid status message",
			"host", stream.Conn().LocalPeer(),
			"peer", stream.Conn().RemotePeer(),
		)

		s.host.peerStatus[stream.Conn().RemotePeer()] = false

		// close connection with peer if status message is not valid
		err := s.host.h.Network().ClosePeer(stream.Conn().RemotePeer())
		if err != nil {
			log.Error("Failed to close peer", "err", err)
		}

	}
}

// ID returns the host id
func (s *Service) ID() string {
	return s.host.id()
}

// Peers returns connected peers
func (s *Service) Peers() []string {
	return PeerIdToStringArray(s.host.h.Network().Peers())
}

// PeerCount returns the number of connected peers
func (s *Service) PeerCount() int {
	return s.host.peerCount()
}

// NoBootstrapping returns true if bootstrapping is disabled, otherwise false
func (s *Service) NoBootstrapping() bool {
	return s.host.noBootstrap
}

// parseMessage reads message from the provided stream
func parseMessage(stream net.Stream) (Message, error) {

	rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))

	// check read byte
	_, err := rw.Reader.ReadByte()
	if err != nil {
		return nil, err
	}

	// decode message
	msg, err := DecodeMessage(rw.Reader)
	if err != nil {
		return nil, err
	}

	return msg, nil
}
