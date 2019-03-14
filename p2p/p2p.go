package p2p

import (
	"bufio"
	"context"
	"fmt"

	ds "github.com/ipfs/go-datastore"
	dsync "github.com/ipfs/go-datastore/sync"
	iaddr "github.com/ipfs/go-ipfs-addr"
	libp2p "github.com/libp2p/go-libp2p"
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
	ctx           context.Context
	host          host.Host
	dht           *kaddht.IpfsDHT
	bootstrapNode string
}

// ServiceConfig is used to initialize a new p2p service
type ServiceConfig struct {
	BootstrapNode string
	Port          int
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

	return &Service{
		ctx:           ctx,
		host:          h,
		dht:           dht,
		bootstrapNode: conf.BootstrapNode,
	}, nil
}

// Start begins the p2p Service, including discovery
func (s *Service) Start() error {
	fmt.Println("Host created. We are:", s.host.ID().Pretty())
	fmt.Println(s.host.Addrs())

	return nil
}

// Stop stops the p2p service
func (s *Service) Stop() {
	// TODO
}

// Broadcast sends a message to all peers
func (s *Service) Broadcast() {
	// TODO
}

// Send sends a message to a specific peer
func (s *Service) Send(peer peer.ID) {
	// TODO
}

// Ping pings a peer
func (s *Service) Ping(peer peer.ID) {
	// TODO
}

func (sc *ServiceConfig) buildOpts() ([]libp2p.Option, error) {
	// TODO: get external ip
	ip := "0.0.0.0"

	addr, err := ma.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", ip, sc.Port))
	if err != nil {
		return nil, err
	}

	return []libp2p.Option{
		libp2p.ListenAddrs(addr),
		libp2p.EnableRelay(),
	}, nil
}

// start DHT discovery
func (s *Service) startDHT() error {
	err := s.dht.Bootstrap(s.ctx)
	if err != nil {
		return err
	}

	addr, err := iaddr.ParseString(s.bootstrapNode)
	if err != nil {
		return err
	}

	peerinfo, err := ps.InfoFromP2pAddr(addr.Multiaddr())
	if err != nil {
		return err
	}

	err = s.host.Connect(s.ctx, *peerinfo)
	return err
}

// TODO: stream handling
func handleStream(stream net.Stream) {
	// Create a buffer stream for non blocking read and write.
	rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))
	str, err := rw.ReadString('\n')
	if err != nil {
		return
	}

	fmt.Println("got stream: ", str)
}
