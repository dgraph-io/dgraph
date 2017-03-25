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

	"github.com/dgraph-io/dgraph/protos/workerp"
	"github.com/dgraph-io/dgraph/x"

	"google.golang.org/grpc"
)

var (
	errNoConnection = fmt.Errorf("No connection exists")
)

// Pool is used to manage the grpc client connections for communicating with
// other worker instances.
type pool struct {
	conns chan *grpc.ClientConn
	Addr  string
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

func (p *poolsi) get(addr string) *pool {
	p.RLock()
	defer p.RUnlock()
	pool, _ := p.all[addr]
	return pool
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

	pool := newPool(addr, 5)
	query := new(workerp.Payload)
	query.Data = make([]byte, 10)
	x.Check2(rand.Read(query.Data))

	conn, err := pool.Get()
	x.Checkf(err, "Unable to connect")

	c := workerp.NewWorkerClient(conn)
	resp, err := c.Echo(context.Background(), query)
	if err != nil {
		log.Printf("While trying to connect to %q, got error: %v\n", addr, err)
		return
	}
	x.AssertTrue(bytes.Equal(resp.Data, query.Data))
	x.Check(pool.Put(conn))
	fmt.Printf("Connection with %q successful.\n", addr)

	p.Lock()
	defer p.Unlock()
	_, has = p.all[addr]
	if has {
		return
	}
	p.all[addr] = pool
}

// NewPool initializes an instance of Pool which is used to connect with other
// workers. The pool instance also has a buffered channel,conn with capacity
// maxCap that stores the connections.
func newPool(addr string, maxCap int) *pool {
	p := new(pool)
	p.Addr = addr
	p.conns = make(chan *grpc.ClientConn, maxCap)
	conn, err := p.dialNew()
	if err != nil {
		log.Fatal(err)
		return nil
	}
	p.conns <- conn
	return p
}

func (p *pool) dialNew() (*grpc.ClientConn, error) {
	return grpc.Dial(p.Addr, grpc.WithInsecure())
}

// Get returns a connection from the pool of connections or a new connection if
// the pool is empty.
func (p *pool) Get() (*grpc.ClientConn, error) {
	if p == nil {
		return nil, errNoConnection
	}

	select {
	case conn := <-p.conns:
		return conn, nil
	default:
		return p.dialNew()
	}
}

// Put returns a connection to the pool or closes and discards the connection
// incase the pool channel is at capacity.
func (p *pool) Put(conn *grpc.ClientConn) error {
	select {
	case p.conns <- conn:
		return nil
	default:
		return conn.Close()
	}
}
