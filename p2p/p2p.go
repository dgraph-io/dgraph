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
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log"
	mrand "math/rand"

	ds "github.com/ipfs/go-datastore"
	dsync "github.com/ipfs/go-datastore/sync"
	libp2p "github.com/libp2p/go-libp2p"
	crypto "github.com/libp2p/go-libp2p-crypto"
	host "github.com/libp2p/go-libp2p-host"
	kaddht "github.com/libp2p/go-libp2p-kad-dht"
	net "github.com/libp2p/go-libp2p-net"
	peer "github.com/libp2p/go-libp2p-peer"
	ps "github.com/libp2p/go-libp2p-peerstore"
	rhost "github.com/libp2p/go-libp2p/p2p/host/routed"
	ma "github.com/multiformats/go-multiaddr"
)

const protocolPrefix = "/polkadot/0.0.0"

// Service defines a p2p service, including host and dht
type Service struct {
	ctx            context.Context
	host           host.Host
	hostAddr       ma.Multiaddr
	dht            *kaddht.IpfsDHT
	bootstrapNodes []*ps.PeerInfo
}

// ServiceConfig is used to initialize a new p2p service
type ServiceConfig struct {
	BootstrapNodes []string
	Port           int
	RandSeed       int64
}

// NewService creates a new p2p.Service using the service config. It initializes the host and dht
func NewService(conf *ServiceConfig) (*Service, error) {
	ctx := context.Background()
	opts, err := conf.buildOpts()
	if err != nil {
		return nil, err
	}

	h, err := libp2p.New(ctx, opts...)
	if err != nil {
		return nil, err
	}
	h.SetStreamHandler(protocolPrefix, handleStream)

	dstore := dsync.MutexWrap(ds.NewMapDatastore())
	dht := kaddht.NewDHT(ctx, h, dstore)

	// wrap the host with routed host so we can look up peers in DHT
	h = rhost.Wrap(h, dht)

	// build host multiaddress
	hostAddr, err := ma.NewMultiaddr(fmt.Sprintf("/ipfs/%s", h.ID().Pretty()))
	if err != nil {
		return nil, err
	}

	bootstrapNodes, err := stringsToPeerInfos(conf.BootstrapNodes)
	s := &Service{
		ctx:            ctx,
		host:           h,
		hostAddr:       hostAddr,
		dht:            dht,
		bootstrapNodes: bootstrapNodes,
	}
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
	if len(s.bootstrapNodes) == 0 {
		e <- errors.New("no peers to bootstrap to")
	}

	// connect to the bootstrap nodes
	err := s.bootstrapConnect()
	if err != nil {
		e <- err
	}

	// Now we can build a full multiaddress to reach this host
	// by encapsulating both addresses:
	addrs := s.host.Addrs()
	log.Println("I can be reached at:")
	for _, addr := range addrs {
		log.Println(addr.Encapsulate(s.hostAddr))
	}

	log.Println("listening for connections...")
	e <- nil
}

// Stop stops the p2p service
func (s *Service) Stop() {
	// TODO
}

// Broadcast sends a message to all peers
func (s *Service) Broadcast(msg []byte) (err error) {
	// TODO
	return nil
}

// Send sends a message to a specific peer
func (s *Service) Send(peer ps.PeerInfo, msg []byte) error {
	err := s.host.Connect(s.ctx, peer)
	if err != nil {
		return err
	}

	stream, err := s.host.NewStream(s.ctx, peer.ID, protocolPrefix)
	if err != nil {
		return err
	}

	_, err = stream.Write(msg)
	if err != nil {
		return err
	}

	return nil
}

// Ping pings a peer
func (s *Service) Ping(peer peer.ID) error {
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

// DHT returns the service's dht
func (s *Service) DHT() *kaddht.IpfsDHT {
	return s.dht
}

// Ctx returns the service's ctx
func (s *Service) Ctx() context.Context {
	return s.ctx
}

func (sc *ServiceConfig) buildOpts() ([]libp2p.Option, error) {
	// TODO: get external ip
	ip := "0.0.0.0"

	priv, err := generateKey(sc.RandSeed)
	if err != nil {
		return nil, err
	}

	addr, err := ma.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", ip, sc.Port))
	if err != nil {
		return nil, err
	}

	return []libp2p.Option{
		libp2p.ListenAddrs(addr),
		libp2p.DisableRelay(),
		libp2p.Identity(priv),
		libp2p.NATPortMap(),
		libp2p.Ping(true),
	}, nil
}

// generateKey generates a libp2p private key which is used for secure messaging
func generateKey(seed int64) (crypto.PrivKey, error) {
	// If the seed is zero, use real cryptographic randomness. Otherwise, use a
	// deterministic randomness source to make generated keys stay the same
	// across multiple runs
	var r io.Reader
	if seed == 0 {
		r = rand.Reader
	} else {
		r = mrand.New(mrand.NewSource(seed))
	}

	// Generate a key pair for this host. We will use it at least
	// to obtain a valid host ID.
	priv, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, r)
	if err != nil {
		return nil, err
	}

	return priv, nil
}

// TODO: message handling
func handleStream(stream net.Stream) {
	defer stream.Close()
	// Create a buffer stream for non blocking read and write.
	rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))
	str, err := rw.ReadString('\n')
	if err != nil {
		return
	}

	fmt.Printf("got stream from %s: %s", stream.Conn().RemotePeer(), str)
	_, err = rw.WriteString("hello friend")
	if err != nil {
		return
	}
}

// PeerCount returns the number of connected peers
func (s *Service) PeerCount() int {
	peers := s.host.Network().Peers()
	return len(peers)
}
