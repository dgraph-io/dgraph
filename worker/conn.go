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

package worker

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"sync"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"

	"google.golang.org/grpc"
)

var (
	errNoConnection = fmt.Errorf("No connection exists")
)

// "pool" is used to manage the grpc client connection for communicating with
// other worker instances.  Right now it just holds one of them.
type pool struct {
	// A "pool" now consists of one connection.  gRPC uses HTTP2 transport to combine
	// messages in the same TCP stream.
	conn *grpc.ClientConn

	Addr string
}

type poolsi struct {
	sync.RWMutex
	all map[string]*pool
}

var pi *poolsi

func init() {
	pi = new(poolsi)
	pi.all = make(map[string]*pool)
}

func pools() *poolsi {
	return pi
}

func (p *poolsi) any() *pool {
	p.RLock()
	defer p.RUnlock()
	for _, pool := range p.all {
		return pool
	}
	return nil
}

func (p *poolsi) get(addr string) (*pool, error) {
	p.RLock()
	defer p.RUnlock()
	pool, ok := p.all[addr]
	if !ok {
		return nil, errNoConnection
	}
	return pool, nil
}

func (p *poolsi) connect(addr string) {
	if addr == *myAddr {
		return
	}
	p.RLock()
	_, has := p.all[addr]
	p.RUnlock()
	if has {
		return
	}

	pool, err := newPool(addr, 5)
	x.Checkf(err, "Unable to connect to host %s", addr)

	p.Lock()
	_, has = p.all[addr]
	if has {
		p.Unlock()
		return
	}
	p.all[addr] = pool
	p.Unlock()

	query := new(protos.Payload)
	query.Data = make([]byte, 10)
	x.Check2(rand.Read(query.Data))

	conn, err := pool.Get()
	x.Checkf(err, "Unable to connect")

	c := protos.NewWorkerClient(conn)
	resp, err := c.Echo(context.Background(), query)
	if err != nil {
		log.Printf("While trying to connect to %q, got error: %v\n", addr, err)
		// Don't return -- let's still put the empty pool in the map.  Its users
		// have to handle errors later anyway.
	} else {
		x.AssertTrue(bytes.Equal(resp.Data, query.Data))
		x.Check(pool.Put(conn))
		fmt.Printf("Connection with %q successful.\n", addr)
	}
}

// NewPool creates a new "pool" with one gRPC connection.
func newPool(addr string, maxCap int) (*pool, error) {
	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	return &pool{conn: conn, Addr: addr}, nil
}

// Get returns a connection to use from the pool of connections.
func (p *pool) Get() (*grpc.ClientConn, error) {
	// TODO: Make p never be nil here.
	if p == nil {
		return nil, errNoConnection
	}

	return p.conn, nil
}

// Put returns a connection to the pool.
func (p *pool) Put(conn *grpc.ClientConn) error {
	// TODO: Get rid of this Put() or not.
	x.AssertTrue(conn == p.conn)
	return nil
}
