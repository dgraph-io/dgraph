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
	"sync/atomic"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"

	"google.golang.org/grpc"
)

var (
	ErrNoConnection    = fmt.Errorf("No connection exists")
	errNoPeerPoolEntry = fmt.Errorf("no peerPool entry")
	errNoPeerPool      = fmt.Errorf("no peerPool pool, could not connect")
)

// "Pool" is used to manage the grpc client connection(s) for communicating with other
// worker instances.  Right now it just holds one of them.
type Pool struct {
	// A "pool" now consists of one connection.  gRPC uses HTTP2 transport to combine
	// messages in the same TCP stream.
	conn *grpc.ClientConn

	Addr string
	// Requires a lock on poolsi.
	// TODO: Remove the refcounting. Let grpc take care of closing connections.
	refcount int64
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

func (p *Pools) Any() (*Pool, error) {
	p.RLock()
	defer p.RUnlock()
	for _, pool := range p.all {
		pool.AddOwner()
		return pool, nil
	}
	return nil, ErrNoConnection
}

func (p *Pools) Get(addr string) (*Pool, error) {
	p.RLock()
	defer p.RUnlock()
	pool, ok := p.all[addr]
	if !ok {
		return nil, ErrNoConnection
	}
	pool.AddOwner()
	return pool, nil
}

// One of these must be called for each call to get(...).
func (p *Pools) Release(pl *Pool) {
	// We close the conn after unlocking p.
	newRefcount := atomic.AddInt64(&pl.refcount, -1)
	if newRefcount == 0 {
		p.Lock()
		delete(p.all, pl.Addr)
		p.Unlock()

		destroyPool(pl)
	}
}

func destroyPool(pl *Pool) {
	err := pl.conn.Close()
	if err != nil {
		x.Printf("Error closing cluster connection: %v\n", err.Error())
	}
}

// Returns a pool that you should call put() on.
func (p *Pools) Connect(addr string) *Pool {
	p.RLock()
	existingPool, has := p.all[addr]
	if has {
		p.RUnlock()
		existingPool.AddOwner()
		return existingPool
	}
	p.RUnlock()

	pool, err := NewPool(addr)
	// TODO: Rename newPool to newConn, rename pool.
	// TODO: This can get triggered with totally bogus config.
	x.Checkf(err, "Unable to connect to host %s", addr)

	p.Lock()
	existingPool, has = p.all[addr]
	if has {
		p.Unlock()
		destroyPool(pool)
		existingPool.refcount++
		return existingPool
	}
	p.all[addr] = pool
	pool.AddOwner() // matches p.put() run by caller
	p.Unlock()

	// No need to block this thread just to print some messages.
	pool.AddOwner() // matches p.put() in goroutine
	go func() {
		defer p.Release(pool)
		err = TestConnection(pool)
		if err != nil {
			x.Printf("Connection to %q fails, got error: %v\n", addr, err)
			// Don't return -- let's still put the empty pool in the map.  Its users
			// have to handle errors later anyway.
		} else {
			x.Printf("Connection with %q healthy.\n", addr)
		}
	}()

	return pool
}

// testConnection tests if we can run an Echo query on a connection.
func TestConnection(p *Pool) error {
	conn := p.Get()

	query := new(protos.Payload)
	query.Data = make([]byte, 10)
	x.Check2(rand.Read(query.Data))

	c := protos.NewRaftClient(conn)
	resp, err := c.Echo(context.Background(), query)
	if err != nil {
		return err
	}
	// If a server is sending bad echos, do we have to freak out and die?
	x.AssertTruef(bytes.Equal(resp.Data, query.Data),
		"non-matching Echo response value from %v", p.Addr)
	return nil
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
	// The pool hasn't been added to poolsi yet, so it gets no refcount.
	return &Pool{conn: conn, Addr: addr, refcount: 0}, nil
}

// Get returns the connection to use from the pool of connections.
func (p *Pool) Get() *grpc.ClientConn {
	return p.conn
}

// AddOwner adds 1 to the refcount for the pool (atomically).
func (p *Pool) AddOwner() {
	atomic.AddInt64(&p.refcount, 1)
}

type PeerPoolEntry struct {
	// Never the empty string.  Possibly a bogus address -- bad port number, the value
	// of *myAddr, or some screwed up Raft config.
	addr string
	// An owning reference to a pool for this peer (or nil if addr is sufficiently bogus).
	poolOrNil *Pool
}

// peerPool stores the peers' addresses and our connections to them.  It has exactly one
// entry for every peer other than ourselves.  Some of these peers might be unreachable or
// have bogus (but never empty) addresses.
type PeerPool struct {
	sync.RWMutex
	peers map[uint64]PeerPoolEntry
}

// getPool returns the non-nil pool for a peer.  This might error even if get(id)
// succeeds, if the pool is nil.  This happens if the peer was configured so badly (it had
// a totally bogus addr) we can't make a pool.  (A reasonable refactoring would have us
// make a pool, one that has a nil gRPC connection.)
//
// You must call pools().release on the pool.
func (p *PeerPool) getPool(id uint64) (*Pool, error) {
	p.RLock()
	defer p.RUnlock()
	ent, ok := p.peers[id]
	if !ok {
		return nil, errNoPeerPoolEntry
	}
	if ent.poolOrNil == nil {
		return nil, errNoPeerPool
	}
	ent.poolOrNil.AddOwner()
	return ent.poolOrNil, nil
}

func (p *PeerPool) get(id uint64) (string, bool) {
	p.RLock()
	defer p.RUnlock()
	ret, ok := p.peers[id]
	return ret.addr, ok
}

func (p *PeerPool) set(id uint64, addr string, pl *Pool) {
	p.Lock()
	defer p.Unlock()
	if old, ok := p.peers[id]; ok {
		if old.poolOrNil != nil {
			Get().Release(old.poolOrNil)
		}
	}
	p.peers[id] = PeerPoolEntry{addr, pl}
}
