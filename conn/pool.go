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

	"github.com/dgraph-io/dgraph/protos/api"
	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/x"

	"google.golang.org/grpc"
)

var (
	ErrNoConnection        = fmt.Errorf("No connection exists")
	ErrUnhealthyConnection = fmt.Errorf("Unhealthy connection")
	errNoPeerPoolEntry     = fmt.Errorf("no peerPool entry")
	errNoPeerPool          = fmt.Errorf("no peerPool pool, could not connect")
	echoDuration           = 10 * time.Second
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
	ticker   *time.Ticker
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
	if !pool.IsHealthy() {
		return nil, ErrUnhealthyConnection
	}
	return pool, nil
}

func (p *Pools) Remove(addr string) {
	p.Lock()
	pool, ok := p.all[addr]
	if !ok {
		p.Unlock()
		return
	}
	delete(p.all, addr)
	p.Unlock()
	pool.close()
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
	x.Printf("== CONNECT ==> Setting %v\n", addr)
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
	pl := &Pool{conn: conn, Addr: addr, lastEcho: time.Now()}
	pl.UpdateHealthStatus()
	go pl.MonitorHealth()
	return pl, nil
}

// Get returns the connection to use from the pool of connections.
func (p *Pool) Get() *grpc.ClientConn {
	p.RLock()
	defer p.RUnlock()
	return p.conn
}

func (p *Pool) close() {
	p.ticker.Stop()
	p.conn.Close()
}

func (p *Pool) UpdateHealthStatus() {
	conn := p.Get()

	query := new(api.Payload)
	query.Data = make([]byte, 10)
	x.Check2(rand.Read(query.Data))

	c := intern.NewRaftClient(conn)
	resp, err := c.Echo(context.Background(), query)
	var lastEcho time.Time
	if err == nil {
		x.AssertTruef(bytes.Equal(resp.Data, query.Data),
			"non-matching Echo response value from %v", p.Addr)
		lastEcho = time.Now()
	} else {
		x.Printf("Echo error from %v. Err: %v\n", p.Addr, err)
	}
	p.Lock()
	p.lastEcho = lastEcho
	p.Unlock()
}

// MonitorHealth monitors the health of the connection via Echo. This function blocks forever.
func (p *Pool) MonitorHealth() {
	p.ticker = time.NewTicker(echoDuration)
	for range p.ticker.C {
		p.UpdateHealthStatus()
	}
}

func (p *Pool) IsHealthy() bool {
	p.RLock()
	defer p.RUnlock()
	return time.Since(p.lastEcho) < 2*echoDuration
}
