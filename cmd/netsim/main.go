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
	"crypto/rand"
	"fmt"
	"os"
	"strconv"
	"time"

	p2p "github.com/ChainSafe/gossamer/p2p"
	log "github.com/ChainSafe/log15"
	peer "github.com/libp2p/go-libp2p-core/peer"
)

var messages = []string{
	"hello friend!\n",
	"i am a node\n",
	"do you want to be friends?\n",
	"ok\n",
	"noot\n",
	"i am a penguin stuck in a computer\n",
	"pls feed me code\n",
}

func sendRandomMessage(s *p2p.Service, peerid peer.ID) error {
	// open new stream with each peer
	ps, err := s.DHT().FindPeer(s.Ctx(), peerid)
	if err != nil {
		return fmt.Errorf("could not find peer: %s", err)
	}

	r := getRandomInt(len(messages))
	msg := messages[r]

	err = s.Send(ps, []byte(msg))
	if err != nil {
		return err
	}

	return nil
}

func getRandomInt(m int) int {
	b := make([]byte, 1)
	_, err := rand.Read(b)
	if err != nil {
		return 0
	}
	r := int(b[0]) % m
	return r
}

func main() {
	if len(os.Args) < 2 {
		log.Crit("please specify number of nodes to start in simulation: go run main.go [num]")
		os.Exit(0)
	}

	num, err := strconv.Atoi(os.Args[1])
	if err != nil {
		log.Crit("main", "error", err)
	}

	sim, err := NewSimulator(num)
	if err != nil {
		log.Crit("NewSimulator", "error", err)
	}

	// TODO: Since we removed IPFS this needs to handle error channel from sim.Bootnode.Stop()
	//defer func() {
	//	err = sim.IPFSNode.Close()
	//	if err != nil {
	//		log.Warn("main", "warn", err.Error())
	//	}
	//}()

	for _, node := range sim.Nodes {
		e := node.Start()
		if <-e != nil {
			log.Warn("main", "start err", err)
		}
	}

	time.Sleep(2 * time.Second)

	for _, node := range sim.Nodes {
		go func(node *p2p.Service) {
			for {
				r := getRandomInt(len(sim.Nodes))

				log.Info("sending message", "from", node.Host().ID(), "to", sim.Nodes[r].Host().ID())

				err = sendRandomMessage(node, sim.Nodes[r].Host().ID())
				if err != nil {
					log.Warn("sending message", "warn", err.Error())
				}

				time.Sleep(3 * time.Second)
			}
		}(node)
	}

	select {}
}
