package p2p

import (
	"context"
	"fmt"
	"time"

	"github.com/ChainSafe/gossamer/common"
	"github.com/ethereum/go-ethereum/log"
	ds "github.com/ipfs/go-datastore"
	dsync "github.com/ipfs/go-datastore/sync"
	"github.com/libp2p/go-libp2p"
	core "github.com/libp2p/go-libp2p-core"
	libp2phost "github.com/libp2p/go-libp2p-core/host"
	net "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	kaddht "github.com/libp2p/go-libp2p-kad-dht"

	rhost "github.com/libp2p/go-libp2p/p2p/host/routed"

	ma "github.com/multiformats/go-multiaddr"

	"github.com/libp2p/go-libp2p/p2p/discovery"
)

const DefaultProtocolId = protocol.ID("/gossamer/dot/0")
const mdnsPeriod = time.Minute

// host is a wrapper around libp2p's host.host
type host struct {
	ctx         context.Context
	h           libp2phost.Host
	hostAddr    ma.Multiaddr
	dht         *kaddht.IpfsDHT
	dhtConfig   kaddht.BootstrapConfig
	bootnodes   []peer.AddrInfo
	noBootstrap bool
	noMdns      bool
	mdns        discovery.Service
	protocolId  protocol.ID
}

func newHost(ctx context.Context, cfg *Config) (*host, error) {
	opts, err := cfg.buildOpts()
	if err != nil {
		return nil, err
	}

	h, err := libp2p.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	protocolId := protocol.ID(cfg.ProtocolId)
	if protocolId == "" {
		protocolId = DefaultProtocolId
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

	dhtConfig := kaddht.BootstrapConfig{
		Queries: 1,
		Period:  time.Second,
	}

	bootstrapNodes, err := stringsToPeerInfos(cfg.BootstrapNodes)
	if err != nil {
		return nil, err
	}

	return &host{
		ctx:         ctx,
		h:           h,
		hostAddr:    hostAddr,
		dht:         dht,
		dhtConfig:   dhtConfig,
		bootnodes:   bootstrapNodes,
		protocolId:  protocolId,
		noBootstrap: cfg.NoBootstrap,
		noMdns:      cfg.NoMdns,
	}, nil

}

// bootstrap connects to all provided bootnodes unless noBootstrap is true
func (h *host) bootstrap() {
	if len(h.bootnodes) == 0 && !h.noBootstrap {
		log.Debug("no peers to bootstrap to")
	}

	for _, peerInfo := range h.bootnodes {
		log.Debug("Attempting to dial peer", "id", peerInfo.ID, "addrs", peerInfo.Addrs)
		err := h.h.Connect(context.Background(), peerInfo)
		if err != nil {
			log.Debug("failed to dial bootstrap peer", "err", err)
		}
	}
}

func (h *host) startMdns() {
	if !h.noMdns {
		mdns, err := discovery.NewMdnsService(h.ctx, h.h, mdnsPeriod, string(h.protocolId))
		if err != nil {
			log.Error("error starting MDNS", "err", err)
		}

		mdns.RegisterNotifee(Notifee{ctx: h.ctx, host: h.h})

		h.mdns = mdns
	}
}

func (h *host) logAddrs() {
	addrs := h.h.Addrs()
	log.Info("You can be reached on the following addresses:")
	for _, addr := range addrs {
		fmt.Println("\t" + addr.Encapsulate(h.hostAddr).String())
	}
}

func (h *host) registerStreamHandler(handler func(net.Stream)) {
	h.h.SetStreamHandler(h.protocolId, handler)
}

func (h *host) connect(addrInfo peer.AddrInfo) (err error) {
	err = h.h.Connect(h.ctx, addrInfo)
	return err
}

// getExistingStream gets an existing stream for a peer that uses the host's protocolId
func (h *host) getExistingStream(p peer.ID) net.Stream {
	conns := h.h.Network().ConnsToPeer(p)
	for _, conn := range conns {
		streams := conn.GetStreams()
		for _, stream := range streams {
			if stream.Protocol() == h.protocolId {
				return stream
			}
		}
	}

	return nil
}

// send propagates a message to a specific peer
func (h *host) send(peer core.PeerAddrInfo, msg []byte) (err error) {
	log.Debug("sending message", "to", peer.ID)

	stream := h.getExistingStream(peer.ID)
	if stream == nil {
		stream, err = h.h.NewStream(h.ctx, peer.ID, h.protocolId)
		log.Debug("opening new stream ", "to", peer.ID)
		if err != nil {
			log.Error("failed to open stream", "error", err)
			return err
		}
	} else {
		log.Debug("using existing stream", "to", peer.ID)
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

func (h *host) broadcast(msg []byte) {
	for _, peers := range h.h.Network().Peers() {
		addrInfo := h.dht.FindLocal(peers)
		_ = h.send(addrInfo, msg)
	}
}

func (h *host) ping(peer core.PeerID) error {
	ps, err := h.dht.FindPeer(h.ctx, peer)
	if err != nil {
		return fmt.Errorf("could not find peer: %s", err)
	}

	err = h.h.Connect(h.ctx, ps)
	if err != nil {
		return err
	}

	return h.dht.Ping(h.ctx, peer)
}

// id returns the ID of the node
func (h *host) id() string {
	return h.h.ID().String()
}

// peerCount returns the number of connected peers
func (h *host) peerCount() int {
	peers := h.h.Network().Peers()
	return len(peers)
}

// close shuts down the host and its components
func (h *host) close() error {
	//Stop the host & IpfsDHT
	err := h.h.Close()
	if err != nil {
		return err
	}

	err = h.dht.Close()
	if err != nil {
		return err
	}

	return nil
}

// fullAddrs returns all the hosts addresses with their ID append as multiaddrs
func (h *host) fullAddrs() (maddrs []ma.Multiaddr) {
	addrs := h.h.Addrs()
	for _, a := range addrs {
		maddr, err := ma.NewMultiaddr(fmt.Sprintf("%s/p2p/%s", a, h.h.ID().Pretty()))
		if err != nil {
			continue
		}
		maddrs = append(maddrs, maddr)
	}
	return maddrs
}
