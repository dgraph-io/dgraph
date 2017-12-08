package xidmap

import (
	"context"
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
	for _, addr := range p.knownAddrs {
		conn, err := p.dial(addr)
		if err != nil {
			x.Printf("could not dial zero addr %q: %v", addr, err)
			continue
		}
		client := intern.NewZeroClient(conn)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		state, err := client.Connect(ctx, &intern.Member{ClusterInfoOnly: true})
		cancel()
		if err != nil {
			x.Printf("could not connect to zero with addr %q: %v", addr, err)
			continue
		}

		p.knownAddrs = p.knownAddrs[:0]
		var leaderAddr string
		for _, member := range state.GetState().Zeros {
			p.knownAddrs = append(p.knownAddrs, member.Addr)
			if member.Leader {
				leaderAddr = member.Addr
			}
		}
		if leaderAddr == "" {
			return nil, x.Errorf("no known zero leader")
		}

		conn, err = p.dial(leaderAddr)
		if err != nil {
			x.Printf("could not dial zero addr %q: %v", leaderAddr, err)
			continue
		}
		return intern.NewZeroClient(conn), nil
	}
	return nil, x.Errorf("could not find leader")
}
