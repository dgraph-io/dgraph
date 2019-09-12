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

package main

import (
	"github.com/ChainSafe/gossamer/p2p"
)

type Simulator struct {
	Nodes    []*p2p.Service
	Bootnode *p2p.Service
}

func NewSimulator(num int) (sim *Simulator, err error) {
	sim = new(Simulator)
	sim.Nodes = make([]*p2p.Service, num)

	bootnodeCfg := &p2p.Config{
		BootstrapNodes: nil,
		Port:           5000,
		RandSeed:       0,
		NoBootstrap:    false,
		NoMdns:         false,
	}

	sim.Bootnode, err = p2p.NewService(bootnodeCfg)
	if err != nil {
		return nil, err
	}

	bootnodeAddr := sim.Bootnode.FullAddrs()[0].String()

	// create all nodes, increment port by 1 each time
	for i := 0; i < num; i++ {
		// configure p2p service
		conf := &p2p.Config{
			BootstrapNodes: []string{
				bootnodeAddr,
			},
			Port: 5001 + i,
		}
		sim.Nodes[i] = new(p2p.Service)
		sim.Nodes[i], err = p2p.NewService(conf)
		if err != nil {
			return nil, err
		}
	}

	return sim, nil
}
