package xidmap

import (
	"context"

	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/x"
	"google.golang.org/grpc"
)

type ZeroPool struct {
	dial  func(addr string) (*grpc.ClientConn, error)
	state *intern.ConnectionState
}

func NewZeroPool(dial func(string) (*grpc.ClientConn, error), addr string) *ZeroPool {
	return &ZeroPool{
		dial: dial,
		state: &intern.ConnectionState{State: &intern.MembershipState{
			Zeros: map[uint64]*intern.Member{0: &intern.Member{Addr: addr}},
		}},
	}
}

func (p *ZeroPool) Leader() (intern.ZeroClient, error) {
	for _, member := range p.state.State.Zeros {
		conn, err := p.dial(member.Addr)
		if err != nil {
			x.Printf("Could not dial zero at address %q: %v", member.Addr, err)
			continue // try next known address
		}

		state, err := intern.NewZeroClient(conn).Connect(
			context.Background(), &intern.Member{ClusterInfoOnly: true})
		if err != nil {
			x.Printf("Could not get membership state from zero at address %q: %v",
				member.Addr, err)
			continue // try next known address
		}

		p.state = state
		var leaderAddr string
		for _, member := range p.state.State.Zeros {
			if member.Leader {
				leaderAddr = member.Addr
				break
			}
		}
		if leaderAddr == "" {
			return nil, x.Errorf("Could not find zero leader")
		}

		conn, err = p.dial(leaderAddr)
		if err != nil {
			return nil, x.Errorf("Could not dial leader zero at address %q: %v",
				leaderAddr, err)
		}
		return intern.NewZeroClient(conn), nil
	}
	return nil, x.Errorf("Failed to obtain zero leader after trying all connections")
}
