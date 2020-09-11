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
	"bufio"
	"context"
	"errors"
	"math/big"
	"os"
	"time"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/services"

	log "github.com/ChainSafe/log15"
	libp2pnetwork "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
)

// NetworkStateTimeout is the set time interval that we update network state
const NetworkStateTimeout = time.Minute
const syncID = "/sync/2"

var _ services.Service = &Service{}
var logger = log.New("pkg", "network")

// Service describes a network service
type Service struct {
	logger log.Logger
	ctx    context.Context
	cancel context.CancelFunc

	cfg            *Config
	host           *host
	mdns           *mdns
	status         *status
	gossip         *gossip
	requestTracker *requestTracker
	errCh          chan<- error

	// Service interfaces
	blockState   BlockState
	networkState NetworkState
	syncer       Syncer

	// Interface for inter-process communication
	messageHandler MessageHandler

	// Configuration options
	noBootstrap bool
	noMDNS      bool
	noStatus    bool // internal option
	noGossip    bool // internal option
}

// NewService creates a new network service from the configuration and message channels
func NewService(cfg *Config) (*Service, error) {
	ctx, cancel := context.WithCancel(context.Background()) //nolint

	h := log.StreamHandler(os.Stdout, log.TerminalFormat())
	h = log.CallerFileHandler(h)
	logger.SetHandler(log.LvlFilterHandler(cfg.LogLvl, h))
	cfg.logger = logger

	// build configuration
	err := cfg.build()
	if err != nil {
		return nil, err //nolint
	}

	if cfg.Syncer == nil {
		return nil, errors.New("cannot have nil Syncer")
	}

	// create a new host instance
	host, err := newHost(ctx, cfg, logger)
	if err != nil {
		return nil, err
	}

	network := &Service{
		logger:         logger,
		ctx:            ctx,
		cancel:         cancel,
		cfg:            cfg,
		host:           host,
		mdns:           newMDNS(host),
		status:         newStatus(host),
		gossip:         newGossip(host),
		requestTracker: newRequestTracker(host.logger),
		blockState:     cfg.BlockState,
		networkState:   cfg.NetworkState,
		messageHandler: cfg.MessageHandler,
		noBootstrap:    cfg.NoBootstrap,
		noMDNS:         cfg.NoMDNS,
		noStatus:       cfg.NoStatus,
		syncer:         cfg.Syncer,
		errCh:          cfg.ErrChan,
	}

	return network, err
}

// Start starts the network service
func (s *Service) Start() error {
	if s.IsStopped() {
		s.ctx, s.cancel = context.WithCancel(context.Background())
	}

	// update network state
	go s.updateNetworkState()

	s.host.registerConnHandler(s.handleConn)
	s.host.registerStreamHandler("", s.handleStream)
	s.host.registerStreamHandler(syncID, s.handleSyncStream)

	// log listening addresses to console
	for _, addr := range s.host.multiaddrs() {
		s.logger.Info("Started listening", "address", addr)
	}

	if !s.noBootstrap {
		s.host.bootstrap()
	}

	// TODO: ensure bootstrap has connected to bootnodes and addresses have been
	// registered by the host before mDNS attempts to connect to bootnodes

	if !s.noMDNS {
		s.mdns.start()
	}

	return nil
}

// Stop closes running instances of the host and network services as well as
// the message channel from the network service to the core service (services that
// are dependent on the host instance should be closed first)
func (s *Service) Stop() error {
	s.cancel()

	// close mDNS discovery service
	err := s.mdns.close()
	if err != nil {
		s.logger.Error("Failed to close mDNS discovery service", "error", err)
	}

	// close host and host services
	err = s.host.close()
	if err != nil {
		s.logger.Error("Failed to close host", "error", err)
	}

	return nil
}

// IsStopped returns true if the service is stopped
func (s *Service) IsStopped() bool {
	return s.ctx.Err() != nil
}

// updateNetworkState updates the network state at the set time interval
func (s *Service) updateNetworkState() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-time.After(NetworkStateTimeout):
			s.networkState.SetHealth(s.Health())
			s.networkState.SetNetworkState(s.NetworkState())
			s.networkState.SetPeers(s.Peers())
		}
	}
}

// SendMessage implementation of interface to handle receiving messages
func (s *Service) SendMessage(msg Message) {
	if s.host == nil {
		return
	}
	if s.IsStopped() {
		return
	}
	if msg == nil {
		s.logger.Debug("Received nil message from core service")
		return
	}
	s.logger.Debug(
		"Broadcasting message from core service",
		"host", s.host.id(),
		"type", msg.Type(),
	)

	// broadcast message to connected peers
	s.host.broadcast(msg)
}

// handleConn starts processes that manage the connection
func (s *Service) handleConn(conn libp2pnetwork.Conn) {
	// check if status is enabled
	if !s.noStatus {

		// get latest block header from block state
		latestBlock, err := s.blockState.BestBlockHeader()
		if err != nil || (latestBlock == nil || latestBlock.Number == nil) {
			s.logger.Error("Failed to get chain head", "error", err)
			return
		}

		// update host status message
		msg := &StatusMessage{
			ProtocolVersion:     s.cfg.ProtocolVersion,
			MinSupportedVersion: s.cfg.MinSupportedVersion,
			Roles:               s.cfg.Roles,
			BestBlockNumber:     latestBlock.Number.Uint64(),
			BestBlockHash:       latestBlock.Hash(),
			GenesisHash:         s.blockState.GenesisHash(),
			ChainStatus:         []byte{0}, // TODO
		}

		// update host status message
		s.status.setHostMessage(msg)

		// manage status messages for new connection
		s.status.handleConn(conn)
	}
}

// handleStream starts reading from the inbound message stream and continues
// reading until the inbound message stream is closed or reset.
func (s *Service) handleStream(stream libp2pnetwork.Stream) {
	conn := stream.Conn()
	if conn == nil {
		s.logger.Error("Failed to get connection from stream")
		return
	}

	peer := conn.RemotePeer()
	s.readStream(stream, peer, s.handleMessage)
	// the stream stays open until closed or reset
}

// handleSyncStream handles streams with the <protcol-id>/sync/2 protocol ID
func (s *Service) handleSyncStream(stream libp2pnetwork.Stream) {
	conn := stream.Conn()
	if conn == nil {
		s.logger.Error("Failed to get connection from stream")
		return
	}

	peer := conn.RemotePeer()
	s.readStream(stream, peer, s.handleSyncMessage)
	// the stream stays open until closed or reset
}

var maxReads = 16

func (s *Service) readStream(stream libp2pnetwork.Stream, peer peer.ID, handler func(peer peer.ID, msg Message)) {

	// create buffer stream for non-blocking read
	r := bufio.NewReader(stream)

	for {
		length, err := readLEB128ToUint64(r)
		if err != nil {
			s.logger.Error("Failed to read LEB128 encoding", "error", err)
			_ = stream.Close()
			s.errCh <- err
			return
		}

		if length == 0 {
			continue
		}

		msgBytes := make([]byte, length)
		tot := uint64(0)
		for i := 0; i < maxReads; i++ {
			n, err := r.Read(msgBytes[tot:]) //nolint
			if err != nil {
				s.logger.Error("Failed to read message from stream", "error", err)
				_ = stream.Close()
				s.errCh <- err
				return
			}

			tot += uint64(n)
			if tot == length {
				break
			}
		}

		if tot != length {
			s.logger.Error("Failed to read entire message", "length", length, "read" /*n*/, tot)
			continue
		}

		// decode message based on message type
		msg, err := decodeMessageBytes(msgBytes)
		if err != nil {
			s.logger.Error("Failed to decode message from peer", "peer", peer, "err", err)
			continue
		}

		s.logger.Trace(
			"Received message from peer",
			"host", s.host.id(),
			"peer", peer,
			"type", msg.Type(),
		)

		// handle message based on peer status and message type
		handler(peer, msg)
	}
}

// handleSyncMessage handles synchronization message types (BlockRequest and BlockResponse)
func (s *Service) handleSyncMessage(peer peer.ID, msg Message) {
	if msg == nil {
		return
	}

	// if it's a BlockResponse with an ID corresponding to a BlockRequest we sent, forward
	// message to the sync service
	if resp, ok := msg.(*BlockResponseMessage); ok && s.requestTracker.hasRequestedBlockID(resp.ID) {
		s.requestTracker.removeRequestedBlockID(resp.ID)
		req := s.syncer.HandleBlockResponse(resp)
		if req != nil {
			s.requestTracker.addRequestedBlockID(req.ID)
			err := s.host.send(peer, syncID, req)
			if err != nil {
				s.logger.Error("failed to send BlockRequest message", "peer", peer)
			}
		}
	}

	// if it's a BlockRequest, call core for processing
	if req, ok := msg.(*BlockRequestMessage); ok {
		resp, err := s.syncer.CreateBlockResponse(req)
		if err != nil {
			s.logger.Debug("cannot create response for request", "id", req.ID)
			return
		}

		err = s.host.send(peer, syncID, resp)
		if err != nil {
			s.logger.Error("failed to send BlockResponse message", "peer", peer)
		}
	}
}

// handleMessage handles the message based on peer status and message type
func (s *Service) handleMessage(peer peer.ID, msg Message) {
	if msg.Type() != StatusMsgType {

		// check if status is disabled or peer status is confirmed
		if s.noStatus || s.status.confirmed(peer) {
			if an, ok := msg.(*BlockAnnounceMessage); ok {
				req := s.syncer.HandleBlockAnnounce(an)
				if req != nil {
					s.requestTracker.addRequestedBlockID(req.ID)
					log.Info("sending", "req", req)
					err := s.host.send(peer, syncID, req)
					if err != nil {
						s.logger.Error("failed to send BlockRequest message", "peer", peer)
					}
				}
			} else {
				if s.messageHandler == nil {
					s.logger.Crit("Failed to send message", "error", "message handler is nil")
					return
				}
				s.messageHandler.HandleMessage(msg)
			}
		}

		// check if gossip is enabled
		if !s.noGossip {

			// handle non-status message from peer with gossip submodule
			s.gossip.handleMessage(msg, peer)
		}

	} else {

		// check if status is enabled
		if !s.noStatus {

			// handle status message from peer with status submodule
			s.status.handleMessage(peer, msg.(*StatusMessage))

			// check if peer status confirmed
			if s.status.confirmed(peer) {

				// send a block request message if peer best block number is greater than host best block number
				req := s.handleStatusMesssage(msg.(*StatusMessage))
				if req != nil {
					s.requestTracker.addRequestedBlockID(req.ID)
					err := s.host.send(peer, syncID, req)
					if err != nil {
						s.logger.Error("failed to send BlockRequest message", "peer", peer)
					}
				}
			}
		}
	}
}

// handleStatusMesssage returns a block request message if peer best block
// number is greater than host best block number
func (s *Service) handleStatusMesssage(statusMessage *StatusMessage) *BlockRequestMessage {
	// get latest block header from block state
	latestHeader, err := s.blockState.BestBlockHeader()
	if err != nil {
		s.logger.Error("Failed to get best block header from block state", "error", err)
		return nil
	}

	bestBlockNum := big.NewInt(int64(statusMessage.BestBlockNumber))

	// check if peer block number is greater than host block number
	if latestHeader.Number.Cmp(bestBlockNum) == -1 {
		s.logger.Debug("sending new block to syncer", "number", statusMessage.BestBlockNumber)
		return s.syncer.HandleSeenBlocks(bestBlockNum)
	}

	return nil
}

// Health returns information about host needed for the rpc server
func (s *Service) Health() common.Health {
	return common.Health{
		Peers:           s.host.peerCount(),
		IsSyncing:       false, // TODO
		ShouldHavePeers: !s.noBootstrap,
	}
}

// NetworkState returns information about host needed for the rpc server and the runtime
func (s *Service) NetworkState() common.NetworkState {
	return common.NetworkState{
		PeerID: s.host.id().String(),
	}
}

// Peers returns information about connected peers needed for the rpc server
func (s *Service) Peers() []common.PeerInfo {
	peers := []common.PeerInfo{}

	for _, p := range s.host.peers() {
		if s.status.confirmed(p) {
			if m, ok := s.status.peerMessage.Load(p); ok {
				msg, ok := m.(*StatusMessage)
				if !ok {
					return peers
				}

				peers = append(peers, common.PeerInfo{
					PeerID:          p.String(),
					Roles:           msg.Roles,
					ProtocolVersion: msg.ProtocolVersion,
					BestHash:        msg.BestBlockHash,
					BestNumber:      msg.BestBlockNumber,
				})
			}
		}
	}
	return peers
}

// NodeRoles Returns the roles the node is running as.
func (s *Service) NodeRoles() byte {
	return s.cfg.Roles
}

//SetMessageHandler sets the given MessageHandler for this service
func (s *Service) SetMessageHandler(handler MessageHandler) {
	s.messageHandler = handler
}
