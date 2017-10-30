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

package main

import (
	"errors"
	"fmt"
	"math/rand"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
	"golang.org/x/net/context"
)

type Oracle struct {
	x.SafeMutex
	commits    map[uint64]uint64 // start -> commit
	rowCommit  map[string]uint64 // fp(key) -> commit
	aborts     map[uint64]struct{}
	pending    map[uint64]struct{}
	maxPending uint64
	// Implement Tmax.
	subscribers map[int]chan *protos.OracleDelta
	updates     chan *protos.TxnContext
}

func (o *Oracle) Init() {
	o.commits = make(map[uint64]uint64)
	o.rowCommit = make(map[string]uint64)
	o.aborts = make(map[uint64]struct{})
	o.pending = make(map[uint64]struct{})
	o.subscribers = make(map[int]chan *protos.OracleDelta)
	o.updates = make(chan *protos.TxnContext, 100000) // Keeping 1 second worth of updates.
	go o.sendDeltasToSubscribers()
}

func (o *Oracle) hasConflict(src *protos.TxnContext) bool {
	for _, k := range src.Keys {
		if last := o.rowCommit[k]; last > src.StartTs {
			return true
		}
	}
	return false
}

func (o *Oracle) commit(src *protos.TxnContext) error {
	o.Lock()
	defer o.Unlock()

	if o.hasConflict(src) {
		return errConflict
	}
	for _, k := range src.Keys {
		o.rowCommit[k] = src.CommitTs // CommitTs is handed out before calling this func.
	}
	return nil
}

func (o *Oracle) currentState() *protos.OracleDelta {
	o.AssertRLock()
	resp := &protos.OracleDelta{}
	for start, commit := range o.commits {
		resp.Commits[start] = commit
	}
	for abort := range o.aborts {
		resp.Aborts = append(resp.Aborts, abort)
	}
	resp.MaxPending = o.maxPending
	return resp
}

func (o *Oracle) newSubscriber() (<-chan *protos.OracleDelta, int) {
	o.Lock()
	defer o.Unlock()
	var id int
	for {
		id = rand.Int()
		if _, has := o.subscribers[id]; !has {
			break
		}
	}
	ch := make(chan *protos.OracleDelta, 1000)
	ch <- o.currentState() // Queue up the full state as the first entry.
	o.subscribers[id] = ch
	return ch, id
}

func (o *Oracle) removeSubscriber(id int) {
	o.Lock()
	defer o.Unlock()
	delete(o.subscribers, id)
}

func (o *Oracle) sendDeltasToSubscribers() {
	delta := new(protos.OracleDelta)
	for {
		tctx, open := <-o.updates
		if !open {
			return
		}
	slurp_loop:
		for {
			// Consume tctx.
			if tctx.Aborted {
				delta.Aborts = append(delta.Aborts, tctx.StartTs)
			} else {
				delta.Commits[tctx.StartTs] = tctx.CommitTs
			}
			select {
			case tctx, open = <-o.updates:
				if !open {
					return
				}
			default:
				break slurp_loop
			}
		}
		o.Lock()
		for id, ch := range o.subscribers {
			select {
			case ch <- delta:
			default:
				close(ch)
				delete(o.subscribers, id)
			}
		}
		o.Unlock()
		delta = new(protos.OracleDelta)
	}
}

func (o *Oracle) updateCommitStatus(src *protos.TxnContext) {
	// TODO: Have a way to clear out the commits and aborts.
	o.updates <- src
	o.Lock()
	defer o.Unlock()
	delete(o.pending, src.StartTs)
	if src.Aborted {
		o.aborts[src.StartTs] = struct{}{}
	} else {
		o.commits[src.StartTs] = src.CommitTs
	}
}

func (o *Oracle) storePending(ids *protos.AssignedIds) {
	o.Lock()
	defer o.Unlock()
	max := ids.EndId - 1
	if o.maxPending < max {
		o.maxPending = max
	}
	for id := ids.StartId; id < ids.EndId; id++ {
		o.pending[id] = struct{}{}
	}
}

var errConflict = errors.New("Transaction conflict")

func (s *Server) proposeTxn(ctx context.Context, src *protos.TxnContext) error {
	var zp protos.ZeroProposal
	zp.Txn = &protos.TxnContext{
		StartTs:  src.StartTs,
		CommitTs: src.CommitTs,
		Aborted:  src.Aborted,
	}
	return s.Node.proposeAndWait(ctx, &zp)
}

func (s *Server) commit(ctx context.Context, src *protos.TxnContext) error {
	if src.Aborted {
		return s.proposeTxn(ctx, src)
	}
	s.orc.RLock()
	conflict := s.orc.hasConflict(src)
	s.orc.RUnlock()
	if conflict {
		src.Aborted = true
		return s.proposeTxn(ctx, src)
	}

	// TODO: Consider Tmax here.
	var num protos.Num
	num.Val = 1
	assigned, err := s.lease(ctx, &num, true)
	if err != nil {
		return err
	}
	src.CommitTs = assigned.StartId

	if err := s.orc.commit(src); err != nil {
		src.Aborted = true
	}
	return s.proposeTxn(ctx, src)
}

func (s *Server) CommitOrAbort(ctx context.Context, src *protos.TxnContext) (*protos.TxnContext, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if !s.Node.AmLeader() {
		return nil, x.Errorf("Only leader can decide to commit or abort")
	}
	err := s.commit(ctx, src)
	return src, err
}

var errClosed = errors.New("Streaming closed by Oracle.")

func (s *Server) Oracle(unused *protos.Payload, server protos.Zero_OracleServer) error {
	ch, id := s.orc.newSubscriber()
	defer s.orc.removeSubscriber(id)

	ctx := server.Context()
	for {
		select {
		case delta, open := <-ch:
			if !open {
				return errClosed
			}
			if err := server.Send(delta); err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
