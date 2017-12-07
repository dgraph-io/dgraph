package xidmap

import (
	"math/rand"
	"sync"

	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/x"
	"google.golang.org/grpc"
)

type ZeroPool struct {
	sync.Mutex
	leaderId   uint32
	leader     intern.ZeroClient
	knownAddrs []string
}

func NewZeroPool(cc *grpc.ClientConn) *ZeroPool {
	zp := &ZeroPool{}
	go func() {
		seedClient := intern.NewZeroClient(cc)
		zp.readMembershipStream(seedClient)
		for {
			client := seedClient
			if len(zp.knownAddrs) > 0 {
				addr := zp.knownAddrs[rand.Intn(len(zp.knownAddrs))]
				err, conn := grpc.Dial(addr)
				if err != nil {
					x.Printf("could not dial zero address %q: %v", addr, err)
					// TODO: Remove from known addresses?
					continue
				}
			}
		}
	}()
	return zp
}

func (p *ZeroPool) Leader() intern.ZeroClient {
	p.Lock()
	defer p.Unlock()
	return p.leader
}

func (p *ZeroPool) readMembershipStream(zc intern.ZeroClient) {
	uc, err := zc.Update(ctx)
	if err != nil {
		x.Printf("could not start reading membership stream: %v", err)
		return
	}
	for {
		ms, err := uc.Recv()
		if err != nil {
			x.Printf("could not read membership stream: %v", err)
			return
		}
		if err := p.updateMembershipState(ms.Zeros); err != nil {
			x.Printf("could not update membership state: %v", err)
			return
		}
	}
}

func (p *ZeroPool) updateMembershipState(state map[uint64]*Member) error {
	p.knownAddrs = p.knownAddrs[:0]
	for id, z := range ms.Zeros {
		p.knownAddrs = append(z.Addr)
		if !z.Leader || p.leaderId == id {
			continue
		}
		p.leaderId = id
		conn, err := grpc.Dial(z.Addr)
		if err != nil {
			return err
		}
		p.leader = zero
	}
	return nil
}
