package api

import (
	log "github.com/ChainSafe/log15"
)

type systemModule struct {
	p2p     P2pApi
	runtime RuntimeApi
}

func NewSystemModule(p2p P2pApi, rt RuntimeApi) *systemModule {
	return &systemModule{
		p2p,
		rt,
	}
}

func (m *systemModule) Version() string {
	log.Debug("[rpc] Executing System.Version", "params", nil)
	return m.runtime.Version()
}

// TODO: Move to 'p2p' module
func (m *systemModule) PeerCount() int {
	log.Debug("[rpc] Executing System.PeerCount", "params", nil)
	return m.p2p.PeerCount()
}
