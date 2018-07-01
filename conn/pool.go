/*
 * Copyright 2016-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package conn

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"sync"
	"time"

	"github.com/dgraph-io/dgo/protos/api"
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
	pool.shutdown()
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
	x.Printf("== CONNECTED ==> Setting %v\n", addr)
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
		grpc.WithBackoffMaxDelay(time.Second),
		grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	pl := &Pool{conn: conn, Addr: addr, lastEcho: time.Now()}
	pl.UpdateHealthStatus(true)

	// Initialize ticker before running monitor health.
	pl.ticker = time.NewTicker(echoDuration)
	go pl.MonitorHealth()
	return pl, nil
}

// Get returns the connection to use from the pool of connections.
func (p *Pool) Get() *grpc.ClientConn {
	p.RLock()
	defer p.RUnlock()
	return p.conn
}

func (p *Pool) shutdown() {
	p.ticker.Stop()
	p.conn.Close()
}

func (p *Pool) UpdateHealthStatus(printError bool) error {
	conn := p.Get()

	query := new(api.Payload)
	query.Data = make([]byte, 10)
	x.Check2(rand.Read(query.Data))

	c := intern.NewRaftClient(conn)
	resp, err := c.Echo(context.Background(), query)
	if err == nil {
		x.AssertTruef(bytes.Equal(resp.Data, query.Data),
			"non-matching Echo response value from %v", p.Addr)
		p.Lock()
		p.lastEcho = time.Now()
		p.Unlock()
	} else if printError {
		x.Printf("Echo error from %v. Err: %v\n", p.Addr, err)
	}
	return err
}

// MonitorHealth monitors the health of the connection via Echo. This function blocks forever.
func (p *Pool) MonitorHealth() {
	var lastErr error
	for range p.ticker.C {
		err := p.UpdateHealthStatus(lastErr == nil)
		if lastErr != nil && err == nil {
			x.Printf("Connection established with %v\n", p.Addr)
		}
		lastErr = err
	}
}

func (p *Pool) IsHealthy() bool {
	p.RLock()
	defer p.RUnlock()
	return time.Since(p.lastEcho) < 2*echoDuration
}
