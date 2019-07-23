package p2p

import (
	"context"

	log "github.com/ChainSafe/log15"
	host "github.com/libp2p/go-libp2p-core/host"
	peer "github.com/libp2p/go-libp2p-core/peer"
	ps "github.com/libp2p/go-libp2p-core/peerstore"
)

// See https://godoc.org/github.com/libp2p/go-libp2p/p2p/discovery#Notifee
type Notifee struct {
	ctx  context.Context
	host host.Host
}

// HandlePeerFound is invoked when a peer in discovered via mDNS
func (n Notifee) HandlePeerFound(p peer.AddrInfo) {
	log.Info("mdns", "peer found", p)

	n.host.Peerstore().AddAddrs(p.ID, p.Addrs, ps.PermanentAddrTTL)
	err := n.host.Connect(n.ctx, p)
	if err != nil {
		log.Error("mdns", "connect error", err)
	}
}
