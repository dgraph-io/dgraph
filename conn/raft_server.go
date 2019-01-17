/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package conn

import (
	"context"
	"encoding/binary"
	"math/rand"
	"sync"

	"github.com/coreos/etcd/raft/raftpb"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
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
	Found uint32
	Ch    chan error
	Ctx   context.Context
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

func (p *proposals) Ctx(key string) context.Context {
	if pctx := p.Get(key); pctx != nil {
		return pctx.Ctx
	}
	return context.Background()
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

func (w *RaftServer) IsPeer(ctx context.Context, rc *pb.RaftContext) (*pb.PeerResponse,
	error) {
	node := w.GetNode()
	if node == nil || node.Raft() == nil {
		return &pb.PeerResponse{}, ErrNoNode
	}

	if node._confState == nil {
		return &pb.PeerResponse{}, nil
	}

	for _, raftIdx := range node._confState.Nodes {
		if rc.Id == raftIdx {
			return &pb.PeerResponse{Status: true}, nil
		}
	}
	return &pb.PeerResponse{}, nil
}

func (w *RaftServer) JoinCluster(ctx context.Context,
	rc *pb.RaftContext) (*api.Payload, error) {
	if ctx.Err() != nil {
		return &api.Payload{}, ctx.Err()
	}
	// Commenting out the following checks for now, until we get rid of groups.
	// TODO: Uncomment this after groups is removed.
	node := w.GetNode()
	if node == nil || node.Raft() == nil {
		return nil, ErrNoNode
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
		return nil, x.Errorf("REUSE_RAFTID: Raft ID duplicates mine: %+v", rc)
	}

	// Check that the new node is not already part of the group.
	if addr, ok := node.Peer(rc.Id); ok && rc.Addr != addr {
		// There exists a healthy connection to server with same id.
		if _, err := Get().Get(addr); err == nil {
			return &api.Payload{}, x.Errorf(
				"REUSE_ADDR: IP Address same as existing peer: %s", addr)
		}
	}
	node.Connect(rc.Id, rc.Addr)

	err := node.AddToCluster(context.Background(), rc.Id)
	glog.Infof("[%#x] Done joining cluster with err: %v", rc.Id, err)
	return &api.Payload{}, err
}

func (w *RaftServer) RaftMessage(ctx context.Context,
	batch *pb.RaftBatch) (*api.Payload, error) {
	if ctx.Err() != nil {
		return &api.Payload{}, ctx.Err()
	}

	rc := batch.GetContext()
	if rc != nil {
		n := w.GetNode()
		if n == nil || n.Raft() == nil {
			return &api.Payload{}, ErrNoNode
		}
		n.Connect(rc.Id, rc.Addr)
	}
	if batch.GetPayload() == nil {
		return &api.Payload{}, nil
	}
	data := batch.Payload.Data
	raft := w.GetNode().Raft()

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
		// This should be done in order, and not via a goroutine.
		if err := raft.Step(ctx, msg); err != nil {
			return &api.Payload{}, err
		}
		idx += sz
	}
	return &api.Payload{}, nil
}

// Hello rpc call is used to check connection with other workers after worker
// tcp server for this instance starts.
func (w *RaftServer) Echo(ctx context.Context, in *api.Payload) (*api.Payload, error) {
	return &api.Payload{Data: in.Data}, nil
}
