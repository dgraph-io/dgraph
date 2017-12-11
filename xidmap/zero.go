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
	knownZeros map[string]intern.ZeroClient
}

func NewZeroPool(dial func(string) (*grpc.ClientConn, error), addr string) *ZeroPool {
	return &ZeroPool{
		dial:       dial,
		knownZeros: map[string]intern.ZeroClient{addr: nil},
	}
}

func (p *ZeroPool) Leader() (intern.ZeroClient, error) {
	for addr := range p.knownZeros {
		zero, err := p.getZero(addr)
		if err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		state, err := zero.Connect(ctx, &intern.Member{ClusterInfoOnly: true})
		cancel()
		if err != nil {
			x.Printf("could not connect to zero with addr %q: %v", addr, err)
			continue
		}

		var leaderAddr string
		for _, member := range state.GetState().Zeros {
			// Adds address to map map of known connections, but doesn't
			// overwrite connection if it already exists.
			p.knownZeros[member.Addr] = p.knownZeros[member.Addr]
			if member.Leader {
				leaderAddr = member.Addr
			}
		}
		if leaderAddr == "" {
			return nil, x.Errorf("no known zero leader")
		}

		zero, err = p.getZero(leaderAddr)
		if err != nil {
			continue
		}
		return zero, nil
	}
	return nil, x.Errorf("could not find leader")
}

func (p *ZeroPool) getZero(addr string) (intern.ZeroClient, error) {
	zero := p.knownZeros[addr]
	if zero != nil {
		return zero, nil
	}
	conn, err := p.dial(addr)
	if err != nil {
		x.Printf("could not dial to zero with address %q: %v", addr, err)
		return nil, err
	}
	zero = intern.NewZeroClient(conn)
	p.knownZeros[addr] = zero
	return zero, nil
}
