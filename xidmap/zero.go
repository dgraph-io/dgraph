package xidmap

import (
	"context"
	"time"

	"github.com/dgraph-io/dgraph/protos/intern"
	"google.golang.org/grpc"
)

type ZeroPool struct {
	dial  func(addr string) (*grpc.ClientConn, error)
	zeros []intern.ZeroClient
	idx   int
}

func NewZeroPool(dial func(string) (*grpc.ClientConn, error), addr string) (*ZeroPool, error) {
	p := &ZeroPool{
		dial: dial,
	}
	conn, err := dial(addr)
	if err != nil {
		return nil, err
	}
	initialZero := intern.NewZeroClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	state, err := initialZero.Connect(ctx, &intern.Member{ClusterInfoOnly: true})
	cancel()
	if err != nil {
		return nil, err
	}
	for _, member := range state.GetState().Zeros {
		conn, err = dial(member.Addr)
		if err != nil {
			return nil, err
		}
		p.zeros = append(p.zeros, intern.NewZeroClient(conn))
		if member.Leader {
			// Use leader first.
			p.idx = len(p.zeros) - 1
		}
	}
	return p, nil
}

func (p *ZeroPool) NextZero() intern.ZeroClient {
	z := p.zeros[p.idx]
	p.idx = (p.idx + 1) % len(p.zeros)
	return z
}
