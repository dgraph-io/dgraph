package xidmap

import (
	"context"
	"time"

	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/x"
	"google.golang.org/grpc"
)

type ZeroPool struct {
	seed       intern.ZeroClient
	knownAddrs []string
}

func NewZeroPool(cc *grpc.ClientConn) *ZeroPool {
	return &ZeroPool{seed: intern.NewZeroClient(cc)}
}

func (p *ZeroPool) Leader() (intern.ZeroClient, error) {
	c := p.Any()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	state, err := c.Connect(ctx, &intern.Member{ClusterInfoOnly: true})
	if err != nil {
		return nil, err
	}
	p.knownAddrs = p.knownAddrs[:0]
	var leaderAddr string
	for _, member := range state.GetState().Zeros {
		p.knownAddrs = append(p.knownAddrs, member.Addr)
		if member.Leader {
			leaderAddr = member.Addr
		}
	}
	conn, err := grpc.Dial(leaderAddr, grpc.WithInsecure())
	return intern.NewZeroClient(conn), err
}

func (p *ZeroPool) Any() intern.ZeroClient {
	for _, addr := range p.knownAddrs {
		conn, err := grpc.Dial(addr, grpc.WithInsecure())
		if err != nil {
			x.Printf("could not dial zero addr %q: %v", addr, err)
			continue
		}
		return intern.NewZeroClient(conn)
	}
	return p.seed
}
