/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package conn

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/rand"
	"sync"

	"github.com/coreos/etcd/raft/raftpb"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/x"
)

type sendmsg struct {
	to   uint64
	data []byte
}

type lockedSource struct {
	lk  sync.Mutex
	src rand.Source
}

func (r *lockedSource) Int63() int64 {
	r.lk.Lock()
	defer r.lk.Unlock()
	return r.src.Int63()
}

func (r *lockedSource) Seed(seed int64) {
	r.lk.Lock()
	defer r.lk.Unlock()
	r.src.Seed(seed)
}

type ProposalCtx struct {
	Ch  chan error
	Ctx context.Context
}

type proposals struct {
	sync.RWMutex
	all map[string]*ProposalCtx
}

func (p *proposals) Store(key string, pctx *ProposalCtx) bool {
	if len(key) == 0 {
		return false
	}
	p.Lock()
	defer p.Unlock()
	if p.all == nil {
		p.all = make(map[string]*ProposalCtx)
	}
	if _, has := p.all[key]; has {
		return false
	}
	p.all[key] = pctx
	return true
}

func (p *proposals) Get(key string) *ProposalCtx {
	p.RLock()
	defer p.RUnlock()
	return p.all[key]
}

func (p *proposals) Delete(key string) {
	if len(key) == 0 {
		return
	}
	p.Lock()
	defer p.Unlock()
	delete(p.all, key)
}

func (p *proposals) Done(key string, err error) {
	if len(key) == 0 {
		return
	}
	p.Lock()
	defer p.Unlock()
	pd, has := p.all[key]
	if !has {
		// If we assert here, there would be a race condition between a context
		// timing out, and a proposal getting applied immediately after. That
		// would cause assert to fail. So, don't assert.
		return
	}
	delete(p.all, key)
	pd.Ch <- err
}

func (w *RaftServer) GetNode() *Node {
	w.nodeLock.RLock()
	defer w.nodeLock.RUnlock()
	return w.Node
}

type RaftServer struct {
	nodeLock sync.RWMutex // protects Node.
	Node     *Node
}

func (w *RaftServer) IsPeer(ctx context.Context, rc *intern.RaftContext) (*intern.PeerResponse,
	error) {
	node := w.GetNode()
	if node == nil || node.Raft() == nil {
		return &intern.PeerResponse{}, errNoNode
	}

	if node._confState == nil {
		return &intern.PeerResponse{}, nil
	}

	for _, raftIdx := range node._confState.Nodes {
		if rc.Id == raftIdx {
			return &intern.PeerResponse{Status: true}, nil
		}
	}
	return &intern.PeerResponse{}, nil
}

func (w *RaftServer) JoinCluster(ctx context.Context,
	rc *intern.RaftContext) (*api.Payload, error) {
	if ctx.Err() != nil {
		return &api.Payload{}, ctx.Err()
	}
	// Commenting out the following checks for now, until we get rid of groups.
	// TODO: Uncomment this after groups is removed.
	node := w.GetNode()
	if node == nil || node.Raft() == nil {
		return nil, errNoNode
	}
	// Only process one JoinCluster request at a time.
	node.joinLock.Lock()
	defer node.joinLock.Unlock()

	// Check that the new node is from the same group as me.
	if rc.Group != node.RaftContext.Group {
		return nil, x.Errorf("Raft group mismatch")
	}
	// Also check that the new node is not me.
	if rc.Id == node.RaftContext.Id {
		return nil, ErrDuplicateRaftId
	}

	// Check that the new node is not already part of the group.
	if addr, ok := node.Peer(rc.Id); ok && rc.Addr != addr {
		// There exists a healthy connection to server with same id.
		if _, err := Get().Get(addr); err == nil {
			return &api.Payload{}, ErrDuplicateRaftId
		}
	}
	node.Connect(rc.Id, rc.Addr)

	err := node.AddToCluster(context.Background(), rc.Id)
	x.Printf("[%d] Done joining cluster with err: %v", rc.Id, err)
	return &api.Payload{}, err
}

var (
	errNoNode = fmt.Errorf("No node has been set up yet")
)

func (w *RaftServer) applyMessage(ctx context.Context, msg raftpb.Message) error {
	node := w.GetNode()
	if node == nil || node.Raft() == nil {
		return errNoNode
	}

	c := make(chan error, 1)
	go func() { c <- node.Raft().Step(ctx, msg) }()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-c:
		return err
	}
}
func (w *RaftServer) RaftMessage(ctx context.Context,
	batch *intern.RaftBatch) (*api.Payload, error) {
	if ctx.Err() != nil {
		return &api.Payload{}, ctx.Err()
	}

	rc := batch.GetContext()
	if rc != nil {
		n := w.GetNode()
		if n == nil {
			return &api.Payload{}, errNoNode
		}
		n.Connect(rc.Id, rc.Addr)
	}
	if batch.GetPayload() == nil {
		return &api.Payload{}, nil
	}
	data := batch.Payload.Data

	for idx := 0; idx < len(data); {
		x.AssertTruef(len(data[idx:]) >= 4,
			"Slice left of size: %v. Expected at least 4.", len(data[idx:]))

		sz := int(binary.LittleEndian.Uint32(data[idx : idx+4]))
		idx += 4
		msg := raftpb.Message{}
		if idx+sz > len(data) {
			return &api.Payload{}, x.Errorf(
				"Invalid query. Specified size %v overflows slice [%v,%v)\n",
				sz, idx, len(data))
		}
		if err := msg.Unmarshal(data[idx : idx+sz]); err != nil {
			x.Check(err)
		}
		if err := w.applyMessage(ctx, msg); err != nil {
			return &api.Payload{}, err
		}
		idx += sz
	}
	// fmt.Printf("Got %d messages\n", count)
	return &api.Payload{}, nil
}

// Hello rpc call is used to check connection with other workers after worker
// tcp server for this instance starts.
func (w *RaftServer) Echo(ctx context.Context, in *api.Payload) (*api.Payload, error) {
	return &api.Payload{Data: in.Data}, nil
}
