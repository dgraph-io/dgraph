package state

import "github.com/ChainSafe/gossamer/p2p"

type networkState struct {
	p2p *p2p.Service
}

func NewNetworkState() *networkState {
	return &networkState{
		// TODO: pass p2p service instance to network state
		p2p: &p2p.Service{},
	}
}

func (ns *networkState) Health() p2p.Health {
	// TODO: return Health() of p2p service
	return p2p.Health{}
}

func (ns *networkState) NetworkState() p2p.NetworkState {
	// TODO: return NetworkState() of p2p service
	return p2p.NetworkState{}
}

func (ns *networkState) Peers() []p2p.PeerInfo {
	// TODO: return Peers() of p2p service
	return []p2p.PeerInfo{}
}
