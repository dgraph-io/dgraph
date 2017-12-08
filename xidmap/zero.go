package xidmap

import (
	"context"
	"fmt"
	"time"

	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/x"
	"google.golang.org/grpc"
)

type ZeroPool struct {
	dial       func(addr string) (*grpc.ClientConn, error)
	knownAddrs []string
}

func NewZeroPool(dial func(string) (*grpc.ClientConn, error), addr string) *ZeroPool {
	return &ZeroPool{
		dial:       dial,
		knownAddrs: []string{addr},
	}
}

func (p *ZeroPool) Leader() (intern.ZeroClient, error) {
	fmt.Println("getting leader")
	for _, addr := range p.knownAddrs {
		fmt.Println("dialing addr:", addr)
		conn, err := p.dial(addr)
		if err != nil {
			x.Printf("could not dial zero addr %q: %v", addr, err)
			continue
		}
		client := intern.NewZeroClient(conn)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		fmt.Println("connecting addr:", addr)
		state, err := client.Connect(ctx, &intern.Member{ClusterInfoOnly: true})
		cancel()
		if err != nil {
			x.Printf("could not connect to zero with addr %q: %v", addr, err)
			continue
		}

		p.knownAddrs = p.knownAddrs[:0]
		var leaderAddr string
		for _, member := range state.GetState().Zeros {
			fmt.Println("member:", member.Addr, member.Leader)
			p.knownAddrs = append(p.knownAddrs, member.Addr)
			if member.Leader {
				leaderAddr = member.Addr
			}
		}

		fmt.Println("dialing:", leaderAddr)
		conn, err = p.dial(leaderAddr)
		if err != nil {
			x.Printf("could not dial zero addr %q: %v", leaderAddr, err)
			continue
		}
		return intern.NewZeroClient(conn), nil
	}
	return nil, x.Errorf("could not find leader")
}
