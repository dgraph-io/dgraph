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
	"errors"
	"fmt"
	core "github.com/libp2p/go-libp2p-core"
	pr "github.com/libp2p/go-libp2p-core/peer"
	ps "github.com/libp2p/go-libp2p-core/peerstore"
	ma "github.com/multiformats/go-multiaddr"
	"log"
	"sync"
)

func stringToPeerInfo(peer string) (*core.PeerAddrInfo, error) {
	maddr := ma.StringCast(peer)
	p, err := pr.AddrInfoFromP2pAddr(maddr)
	return p, err
}

func stringsToPeerInfos(peers []string) ([]*core.PeerAddrInfo, error) {
	pinfos := make([]*core.PeerAddrInfo, len(peers))
	for i, peer := range peers {
		p, err := stringToPeerInfo(peer)
		if err != nil {
			return nil, err
		}
		pinfos[i] = p
	}
	return pinfos, nil
}

// this code is borrowed from the go-ipfs bootstrap process
func (s *Service) bootstrapConnect() error {
	peers := s.bootstrapNodes
	if len(peers) < 1 {
		return errors.New("not enough bootstrap peers")
	}

	// begin bootstrapping
	errs := make(chan error, len(peers))
	var wg sync.WaitGroup

	var err error
	for _, p := range peers {

		// performed asynchronously because when performed synchronously, if
		// one `Connect` call hangs, subsequent calls are more likely to
		// fail/abort due to an expiring context.

		wg.Add(1)
		go func(p *core.PeerAddrInfo) {
			defer wg.Done()
			defer log.Println(s.ctx, "bootstrapDial", s.host.ID(), p.ID)
			log.Printf("%s bootstrapping to %s", s.host.ID(), p.ID)

			s.host.Peerstore().AddAddrs(p.ID, p.Addrs, ps.PermanentAddrTTL)
			if err = s.host.Connect(s.ctx, *p); err != nil {
				log.Println(s.ctx, "bootstrapDialFailed", p.ID)
				log.Printf("failed to bootstrap with %v: %s", p.ID, err)
				errs <- err
				return
			}
			log.Println(s.ctx, "bootstrapDialSuccess", p.ID)
			log.Printf("bootstrapped with %v", p.ID)
		}(p)
	}
	wg.Wait()

	// our failure condition is when no connection attempt succeeded.
	// drain the errs channel, counting the results.
	close(errs)
	count := 0
	for err = range errs {
		if err != nil {
			count++
		}
	}
	if count == len(peers) {
		return fmt.Errorf("failed to bootstrap. %s", err)
	}
	return err
}
