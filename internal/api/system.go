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
