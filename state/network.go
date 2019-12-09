package state

import "github.com/ChainSafe/gossamer/p2p"

type networkState struct {
	peer *p2p.Service
}

func NewNetworkState() *networkState {
	return &networkState{
		peer: &p2p.Service{},
	}
}

func (ns *networkState) Peers() []string {
	return ns.peer.Peers()
}

func (ns *networkState) State() string {
	// TODO: return the network's state
	return ""
}

func (ns *networkState) PeerCount() int {
	return ns.peer.PeerCount()
}

func (ns *networkState) Status() string {
	// TODO: return the network'status
	return ""
}
