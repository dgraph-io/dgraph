package main

import (
	"crypto/rand"
	"fmt"
	"os"
	"strconv"
	"time"

	p2p "github.com/ChainSafe/gossamer/p2p"
	log "github.com/inconshreveable/log15"
	peer "github.com/libp2p/go-libp2p-peer"
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
		log.Crit("please specify number of nodes to start in simulation: ./p2p/simulator/main.go [num]")
		os.Exit(0)
	}

	num, err := strconv.Atoi(os.Args[1])
	if err != nil {
		log.Crit("main", "error", err)
	}

	sim, err := p2p.NewSimulator(num)
	if err != nil {
		log.Crit("NewSimulator", "error", err)
	}

	defer func() {
		err = sim.IpfsNode.Close()
		if err != nil {
			log.Warn("main", "warn", err.Error())
		}
	}()

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
