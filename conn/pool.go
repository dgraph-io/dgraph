/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package conn

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"sync"
	"time"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"

	"google.golang.org/grpc"
)

var (
	ErrNoConnection    = fmt.Errorf("No connection exists")
	errNoPeerPoolEntry = fmt.Errorf("no peerPool entry")
	errNoPeerPool      = fmt.Errorf("no peerPool pool, could not connect")
	echoDuration       = time.Minute
)

// "Pool" is used to manage the grpc client connection(s) for communicating with other
// worker instances.  Right now it just holds one of them.
type Pool struct {
	sync.RWMutex
	// A "pool" now consists of one connection.  gRPC uses HTTP2 transport to combine
	// messages in the same TCP stream.
	conn *grpc.ClientConn

	lastEcho time.Time
	Addr     string
}

type Pools struct {
	sync.RWMutex
	all map[string]*Pool
}

var pi *Pools

func init() {
	pi = new(Pools)
	pi.all = make(map[string]*Pool)
}

func Get() *Pools {
	return pi
}

func (p *Pools) Get(addr string) (*Pool, error) {
	p.RLock()
	defer p.RUnlock()
	pool, ok := p.all[addr]
	if !ok {
		return nil, ErrNoConnection
	}
	return pool, nil
}

func (p *Pools) Connect(addr string) *Pool {
	p.RLock()
	existingPool, has := p.all[addr]
	if has {
		p.RUnlock()
		return existingPool
	}
	p.RUnlock()

	pool, err := NewPool(addr)
	if err != nil {
		x.Printf("Unable to connect to host: %s", addr)
		return nil
	}

	p.Lock()
	existingPool, has = p.all[addr]
	if has {
		p.Unlock()
		return existingPool
	}
	p.all[addr] = pool
	p.Unlock()
	return pool
}

// NewPool creates a new "pool" with one gRPC connection, refcount 0.
func NewPool(addr string) (*Pool, error) {
	conn, err := grpc.Dial(addr,
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(x.GrpcMaxSize),
			grpc.MaxCallSendMsgSize(x.GrpcMaxSize)),
		grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	pl := &Pool{conn: conn, Addr: addr}
	go pl.MonitorHealth()
	return pl, nil
}

// Get returns the connection to use from the pool of connections.
func (p *Pool) Get() *grpc.ClientConn {
	p.RLock()
	defer p.RUnlock()
	return p.conn
}

// MonitorHealth monitors the health of the connection via Echo. This function blocks forever.
func (p *Pool) MonitorHealth() {
	ticker := time.NewTicker(echoDuration)
	for range ticker.C {
		conn := p.Get()

		query := new(protos.Payload)
		query.Data = make([]byte, 10)
		x.Check2(rand.Read(query.Data))

		c := protos.NewRaftClient(conn)
		resp, err := c.Echo(context.Background(), query)
		if err == nil {
			x.AssertTruef(bytes.Equal(resp.Data, query.Data),
				"non-matching Echo response value from %v", p.Addr)
			p.Lock()
			p.lastEcho = time.Now()
			p.Unlock()
		}
	}
}

func (p *Pool) IsHealthy() bool {
	p.RLock()
	defer p.RUnlock()
	return time.Since(p.lastEcho) < 3*echoDuration
}
