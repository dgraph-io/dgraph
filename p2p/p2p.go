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
	"errors"
	"fmt"
	"time"

	"github.com/ChainSafe/gossamer/common"
	log "github.com/ChainSafe/log15"
	ds "github.com/ipfs/go-datastore"
	dsync "github.com/ipfs/go-datastore/sync"
	libp2p "github.com/libp2p/go-libp2p"
	core "github.com/libp2p/go-libp2p-core"
	host "github.com/libp2p/go-libp2p-core/host"
	net "github.com/libp2p/go-libp2p-core/network"
	peer "github.com/libp2p/go-libp2p-core/peer"
	kaddht "github.com/libp2p/go-libp2p-kad-dht"
	discovery "github.com/libp2p/go-libp2p/p2p/discovery"
	rhost "github.com/libp2p/go-libp2p/p2p/host/routed"
	ma "github.com/multiformats/go-multiaddr"
)

const ProtocolPrefix = "/substrate/dot/2"
const mdnsPeriod = time.Minute

// Service describes a p2p service, including host and dht
type Service struct {
	ctx              context.Context
	host             core.Host
	hostAddr         ma.Multiaddr
	dht              *kaddht.IpfsDHT
	dhtConfig        kaddht.BootstrapConfig
	bootstrapNodes   []peer.AddrInfo
	mdns             discovery.Service
	msgChan          chan<- Message
	noBootstrap      bool
	blockReqRec      map[string]bool
	blockRespRec     map[string]bool
	blockAnnounceRec map[string]bool
	txMessageRec     map[string]bool
}

// NewService creates a new p2p.Service using the service config. It initializes the host and dht
func NewService(conf *Config, msgChan chan<- Message) (*Service, error) {
	ctx := context.Background()
	opts, err := conf.buildOpts()
	if err != nil {
		return nil, err
	}

	h, err := libp2p.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	dstore := dsync.MutexWrap(ds.NewMapDatastore())
	dht := kaddht.NewDHT(ctx, h, dstore)

	// wrap the host with routed host so we can look up peers in DHT
	h = rhost.Wrap(h, dht)

	// build host multiaddress
	hostAddr, err := ma.NewMultiaddr(fmt.Sprintf("/p2p/%s", h.ID().Pretty()))
	if err != nil {
		return nil, err
	}

	var mdns discovery.Service
	if !conf.NoMdns {
		mdns, err = discovery.NewMdnsService(ctx, h, mdnsPeriod, ProtocolPrefix)
		if err != nil {
			return nil, err
		}

		mdns.RegisterNotifee(Notifee{ctx: ctx, host: h})
	}

	dhtConfig := kaddht.BootstrapConfig{
		Queries: 1,
		Period:  time.Second,
	}

	bootstrapNodes, err := stringsToPeerInfos(conf.BootstrapNodes)
	s := &Service{
		ctx:            ctx,
		host:           h,
		hostAddr:       hostAddr,
		dht:            dht,
		dhtConfig:      dhtConfig,
		bootstrapNodes: bootstrapNodes,
		noBootstrap:    conf.NoBootstrap,
		mdns:           mdns,
		msgChan:        msgChan,
	}

	s.blockReqRec = make(map[string]bool)
	s.blockRespRec = make(map[string]bool)
	s.blockAnnounceRec = make(map[string]bool)
	s.txMessageRec = make(map[string]bool)

	h.SetStreamHandler(ProtocolPrefix, s.handleStream)

	return s, err
}

// Start begins the p2p Service, including discovery
func (s *Service) Start() <-chan error {
	e := make(chan error)
	go s.start(e)
	return e
}

// start begins the p2p Service, including discovery. start does not terminate once called.
func (s *Service) start(e chan error) {
	if len(s.bootstrapNodes) == 0 && !s.noBootstrap {
		e <- errors.New("no peers to bootstrap to")
	}

	// this is in a go func that loops every minute due to the fact that we appear
	// to get kicked off the network after a few minutes
	// this will likely be resolved once we send messages back to the network
	go func() {
		for {
			if !s.noBootstrap {
				// connect to the bootstrap nodes
				err := s.bootstrapConnect()
				if err != nil {
					e <- err
				}
			}

			err := s.dht.Bootstrap(s.ctx)
			if err != nil {
				e <- err
			}
			time.Sleep(time.Minute)
		}
	}()

	// Now we can build a full multiaddress to reach this host
	// by encapsulating both addresses:
	addrs := s.host.Addrs()
	log.Info("You can be reached on the following addresses:")
	for _, addr := range addrs {
		log.Info(addr.Encapsulate(s.hostAddr).String())
	}

	log.Info("listening for connections...")
	e <- nil
}

// Stop stops the p2p service
func (s *Service) Stop() <-chan error {
	e := make(chan error)

	//Stop the host & IpfsDHT
	err := s.host.Close()
	if err != nil {
		e <- err
	}

	err = s.dht.Close()
	if err != nil {
		e <- err
	}

	return e
}

// Broadcast sends a message to all peers
func (s *Service) Broadcast(msg Message) (err error) {
	//If the node hasn't received the message yet, add it to a list of received messages & rebroadcast it
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
		log.Error("Can't decode message type")
		return
	}

	decodedMsg, err := msg.Encode()
	if err != nil {
		log.Error("Can't encode message")
	}

	for _, peers := range s.host.Network().Peers() {
		addrInfo := s.dht.FindLocal(peers)
		err = s.Send(addrInfo, decodedMsg)
	}

	return err
}

// Send sends a message to a specific peer
func (s *Service) Send(peer core.PeerAddrInfo, msg []byte) (err error) {
	log.Debug("sending message", "peer", peer.ID, "msg", fmt.Sprintf("0x%x", msg))

	stream := s.getExistingStream(peer.ID)
	if stream == nil {
		stream, err = s.host.NewStream(s.ctx, peer.ID, ProtocolPrefix)
		log.Debug("opening new stream ", "peer", peer.ID)
		if err != nil {
			log.Error("failed to open stream", "error", err)
			return err
		}
	} else {
		log.Debug("using existing stream", "peer", peer.ID)
	}

	// Write length of message, and then message
	_, err = stream.Write(common.Uint16ToBytes(uint16(len(msg)))[0:1])
	if err != nil {
		log.Error("fail to send message", "error", err)
		return err
	}
	_, err = stream.Write(msg)
	if err != nil {
		log.Error("fail to send message", "error", err)
		return err
	}

	return nil
}

// Ping pings a peer
func (s *Service) Ping(peer core.PeerID) error {
	ps, err := s.dht.FindPeer(s.ctx, peer)
	if err != nil {
		return fmt.Errorf("could not find peer: %s", err)
	}

	err = s.host.Connect(s.ctx, ps)
	if err != nil {
		return err
	}

	return s.dht.Ping(s.ctx, peer)
}

// Host returns the service's host
func (s *Service) Host() host.Host {
	return s.host
}

// FullAddrs returns all the hosts addresses with their ID append as multiaddrs
func (s *Service) FullAddrs() (maddrs []ma.Multiaddr) {
	addrs := s.host.Addrs()
	for _, a := range addrs {
		maddr, err := ma.NewMultiaddr(fmt.Sprintf("%s/p2p/%s", a, s.host.ID().Pretty()))
		if err != nil {
			continue
		}
		maddrs = append(maddrs, maddr)
	}
	return maddrs
}

// DHT returns the service's dht
func (s *Service) DHT() *kaddht.IpfsDHT {
	return s.dht
}

// Ctx returns the service's ctx
func (s *Service) Ctx() context.Context {
	return s.ctx
}

// PeerCount returns the number of connected peers
func (s *Service) PeerCount() int {
	peers := s.host.Network().Peers()
	return len(peers)
}

// getExistingStream gets an existing stream for a peer that uses ProtocolPrefix
func (s *Service) getExistingStream(p peer.ID) net.Stream {
	conns := s.host.Network().ConnsToPeer(p)
	for _, conn := range conns {
		streams := conn.GetStreams()
		for _, stream := range streams {
			if stream.Protocol() == ProtocolPrefix {
				return stream
			}
		}
	}

	return nil
}

// handles stream; reads message length, message type, and decodes message based on type
// TODO: implement all message types; send message back to peer when we get a message; gossip for certain message types
func (s *Service) handleStream(stream net.Stream) {
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
		return
	}

	// decode message length using LEB128
	length := LEB128ToUint64([]byte{lengthByte})

	// read message type byte
	msgType, err := rw.Reader.Peek(1)
	if err != nil {
		log.Error("failed to read message type", "err", err)
		return
	}

	// read entire message
	rawMsg, err := rw.Reader.Peek(int(length) - 1)
	if err != nil {
		log.Error("failed to read message", "err", err)
		return
	}

	log.Debug("got stream", "peer", stream.Conn().RemotePeer(), "msg", fmt.Sprintf("0x%x", rawMsg))

	// decode message
	msg, err := DecodeMessage(rw.Reader)
	if err != nil {
		log.Error("failed to decode message", "error", err)
		return
	}

	log.Debug("got message", "peer", stream.Conn().RemotePeer(), "type", msgType, "msg", msg.String())

	// Rebroadcast all messages except for status messages
	if msg.GetType() != StatusMsgType {
		err = s.Broadcast(msg)
		if err != nil {
			log.Debug("failed to broadcast message: ", err)
			return
		}
	}

	s.msgChan <- msg
}

// Peers returns connected peers
func (s *Service) Peers() []string {
	return PeerIdToStringArray(s.host.Network().Peers())
}

// ID returns the ID of the node
func (s *Service) ID() string {
	return s.host.ID().String()
}

// NoBootstrapping returns true if you can't bootstrap nodes
func (s *Service) NoBootstrapping() bool {
	return s.noBootstrap
}
