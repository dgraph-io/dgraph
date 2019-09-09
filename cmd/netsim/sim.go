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
