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
	"fmt"

	module "github.com/ChainSafe/gossamer/internal/api/modules"
	"github.com/ChainSafe/gossamer/internal/services"
	log "github.com/ChainSafe/log15"

	net "github.com/libp2p/go-libp2p-core/network"
)

var _ services.Service = &Service{}

// Service describes a p2p service, including host and dht
type Service struct {
	ctx              context.Context
	host             *host
	msgSend          chan<- Message
	msgRec           <-chan Message
	blockReqRec      map[string]bool
	blockRespRec     map[string]bool
	blockAnnounceRec map[string]bool
	txMessageRec     map[string]bool
}

// NewService creates a new p2p.Service using the service config. It initializes the host and dht
func NewService(conf *Config, msgSend chan<- Message, msgRec <-chan Message) (*Service, error) {
	ctx := context.Background()

	host, err := newHost(ctx, conf)
	if err != nil {
		return nil, err
	}

	s := &Service{
		ctx:     ctx,
		host:    host,
		msgSend: msgSend,
		msgRec:  msgRec,
	}

	host.registerStreamHandler(s.handleStream)

	s.blockReqRec = make(map[string]bool)
	s.blockRespRec = make(map[string]bool)
	s.blockAnnounceRec = make(map[string]bool)
	s.txMessageRec = make(map[string]bool)

	return s, err
}

// Start begins the p2p Service, including discovery
func (s *Service) Start() error {
	s.host.startMdns()
	s.host.bootstrap()
	s.host.logAddrs()

	log.Info("Listening for connections...")

	log.Debug("Starting Message Polling for Block Announce Messages from BABE")

	e := make(chan error)

	go s.MsgRecPoll(e)

	return nil
}

// Stop stops the p2p service
func (s *Service) Stop() error {
	err := s.host.close()
	if err != nil {
		log.Error("error closing host", "err", err)
	}

	if s.msgSend != nil {
		close(s.msgSend)
	}

	return nil
}

// MsgRecPoll starts polling the msgRec channel for any blocks
func (s *Service) MsgRecPoll(e chan error) {
	for {
		msg := <-s.msgRec

		host := s.host.hostAddr
		log.Info("received message", "host", host, "message", msg)

		err := s.Broadcast(msg)
		if err != nil {
			e <- err
			break
		}
	}
}

// Broadcast sends a message to all peers
func (s *Service) Broadcast(msg Message) (err error) {
	// If the node hasn't received the message yet, add it to a list of received messages & rebroadcast it
	msgType := msg.GetType()

	switch msgType {
	case BlockRequestMsgType:
		if s.blockReqRec[msg.Id()] {
			return nil
		}
		s.blockReqRec[msg.Id()] = true
	case BlockResponseMsgType:
		if s.blockRespRec[msg.Id()] {
			return nil
		}
		s.blockRespRec[msg.Id()] = true
	case BlockAnnounceMsgType:
		if s.blockAnnounceRec[msg.Id()] {
			return nil
		}
		s.blockAnnounceRec[msg.Id()] = true
	case TransactionMsgType:
		if s.txMessageRec[msg.Id()] {
			return nil
		}
		s.txMessageRec[msg.Id()] = true
	default:
		log.Error("Invalid message type", "type", msgType)
		return nil
	}

	encodedMsg, err := msg.Encode()
	if err != nil {
		return err
	}

	s.host.broadcast(encodedMsg)

	return err
}

// handleStream handles the stream, and rebroadcasts the message based on it's type
func (s *Service) handleStream(stream net.Stream) {
	msg, _, err := parseMessage(stream)
	if err != nil {
		return
	}

	// Write the message back to channel
	s.msgSend <- msg

	// Rebroadcast all messages except for status messages
	if msg.GetType() != StatusMsgType {
		err = s.Broadcast(msg)
		if err != nil {
			log.Debug("failed to broadcast message: ", err)
			return
		}
	}
}

var _ module.P2pApi = &Service{}

// ID returns the host's ID
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

// parseMessage reads message length, message type, decodes message based on type, and returns the decoded message
func parseMessage(stream net.Stream) (Message, []byte, error) {
	defer func() {
		if err := stream.Close(); err != nil {
			log.Error("fail to close stream", "error", err)
		}
	}()

	log.Debug("got stream", "peer", stream.Conn().RemotePeer())

	rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))
	lengthByte, err := rw.Reader.ReadByte()
	if err != nil {
		log.Error("failed to read message length", "peer", stream.Conn().RemotePeer(), "error", err)
		return nil, nil, err
	}

	// decode message length using LEB128
	length := LEB128ToUint64([]byte{lengthByte})

	// read message type byte
	msgType, err := rw.Reader.Peek(1)
	if err != nil {
		log.Error("failed to read message type", "err", err)
		return nil, nil, err
	}

	// read entire message
	rawMsg, err := rw.Reader.Peek(int(length))
	if err != nil {
		log.Error("failed to read message", "err", err)
		return nil, nil, err
	}

	log.Debug("got stream", "peer", stream.Conn().RemotePeer(), "msg", fmt.Sprintf("0x%x", rawMsg))

	// decode message
	msg, err := DecodeMessage(rw.Reader)
	if err != nil {
		log.Error("failed to decode message", "error", err)
		return nil, nil, err
	}

	log.Debug("got message", "peer", stream.Conn().RemotePeer(), "type", msgType, "msg", msg.String())

	return msg, rawMsg, nil
}
