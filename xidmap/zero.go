package xidmap

import (
	"context"
	"sync"

	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/x"
	"google.golang.org/grpc"
)

type ZeroPool struct {
	sync.Mutex
	leader intern.ZeroClient
	zeros  map[string]intern.ZeroClient
}

func NewZero(cc *grpc.ClientConn) *ZeroPool {
	p := &ZeroPool{}
	go func() {
		c := intern.NewZeroClient(cc)
		stream, err := c.Update(context.Background())
		x.Check(err) // TODO: Retry?
		for {
			ms, err := stream.Recv()
			// TODO: Handle error
			for _, zz := range ms.Zeros {
				if zz.Leader {
					p.Lock()
					if cl, ok := p.zeros[zz.Addr]; ok {
					}

					zz.Addr
					break
				}
			}
		}
	}()
	return p
}

func (p *ZeroPool) Leader() intern.ZeroClient {
	p.Lock()
	defer p.Unlock()
	return p.leader
}
