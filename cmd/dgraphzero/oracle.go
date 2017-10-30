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
	"sync"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
	"golang.org/x/net/context"
)

type Oracle struct {
	sync.RWMutex
	commits   map[uint64]uint64 // start -> commit
	rowCommit map[string]uint64 // fp(key) -> commit
	aborts    map[uint64]struct{}
	pending   map[uint64]struct{}
	// Implement Tmax.
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

func (o *Oracle) updateCommitStatus(src *protos.TxnContext) {
	// TODO: Send this out to all the subscribers.
	// TODO: Have a way to clear out the commits and aborts.
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

func (s *Server) Oracle(unused *protos.Payload, server protos.Zero_OracleServer) error {
	return nil
}
