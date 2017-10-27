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
	"errors"
	"fmt"
	"sync"

	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
	farm "github.com/dgryski/go-farm"
)

type task struct {
	rid  uint64 // raft index corresponding to the task
	pid  uint32 // proposal id corresponding to the task
	edge *protos.DirectedEdge
}

type scheduler struct {
	sync.Mutex
	// stores the list of tasks per hash of subject,predicate. Even
	// if there is collision it would create fake dependencies but
	// the end result would be logically correct
	tasks map[uint32][]*task
	tch   chan *task

	n *node
}

func (s *scheduler) init(n *node) {
	s.n = n
	s.tasks = make(map[uint32][]*task)
	s.tch = make(chan *task, 1000)
	for i := 0; i < 1000; i++ {
		go s.processTasks()
	}
}

func (s *scheduler) processTasks() {
	n := s.n
	for t := range s.tch {
		nextTask := t
		for nextTask != nil {
			err := s.n.processMutation(nextTask)
			if err == posting.ErrRetry {
				continue
			}
			n.props.Done(nextTask.pid, err)
			x.ActiveMutations.Add(-1)
			nextTask = s.nextTask(nextTask)
		}
	}
}

func (t *task) key() uint32 {
	key := fmt.Sprintf("%s|%d", t.edge.Attr, t.edge.Entity)
	return farm.Fingerprint32([]byte(key))
}

func (s *scheduler) register(t *task) bool {
	s.Lock()
	defer s.Unlock()
	key := t.key()

	if tasks, ok := s.tasks[key]; ok {
		tasks = append(tasks, t)
		s.tasks[key] = tasks
		return false
	} else {
		tasks = []*task{t}
		s.tasks[key] = tasks
		return true
	}
}

// We don't support schema mutations across nodes in a transaction.
// Wait for all transactions to either abort or complete and all write transactions
// involving the predicate are aborted until schema mutations are done.
func (s *scheduler) schedule(proposal *protos.Proposal, index uint64) (err error) {
	defer func() {
		s.n.props.Done(proposal.Id, err)
		s.n.Applied.WaitForMark(context.Background(), index)
	}()

	if proposal.Mutations.DropAll {
		if err = s.n.Applied.WaitForMark(s.n.ctx, index-1); err != nil {
			return err
		}
		schema.State().DeleteAll()
		err = posting.DeleteAll()
		posting.Txns().Reset()
		posting.SyncMarks().Done(index)
		return
	}

	if len(proposal.Mutations.Schema) > 0 {
		if err = s.n.Applied.WaitForMark(s.n.ctx, index-1); err != nil {
			return err
		}
		startTs := proposal.Mutations.StartTs
		if startTs == 0 {
			return errors.New("StartTs must be provided.")
		}
		for _, supdate := range proposal.Mutations.Schema {
			// This is neceassry to ensure that there is no race between when we start reading
			// from badger and new mutation getting commited via raft and getting applied.
			// Before Moving the predicate we would flush all and wait for watermark to catch up
			// but there might be some proposals which got proposed but not comitted yet.
			// It's ok to reject the proposal here and same would happen on all nodes because we
			// would have proposed membershipstate, and all nodes would have the proposed state
			// or some state after that before reaching here.
			if tablet := groups().Tablet(supdate.Predicate); tablet != nil && tablet.ReadOnly {
				err = errPredicateMoving
				break
			}
			tctxs := posting.Txns().Iterate(func(key []byte) bool {
				pk := x.Parse(key)
				return pk.Attr == supdate.Predicate && (pk.IsIndex() || pk.IsCount() || pk.IsReverse())
			})
			if len(tctxs) > 0 {
				go fixConflicts(tctxs)
				err = posting.ErrConflict
			} else {
				err = s.n.processSchemaMutations(proposal.Id, index, startTs, supdate)
			}
			if err != nil {
				break
			}
		}
		posting.SyncMarks().Done(index)
		return
	}

	// Scheduler tracks tasks at subject, predicate level, so doing
	// schema stuff here simplies the design and we needn't worry about
	// serializing the mutations per predicate or schema mutations
	// We derive the schema here if it's not present
	// Since raft committed logs are serialized, we can derive
	// schema here without any locking

	// stores a map of predicate and type of first mutation for each predicate
	schemaMap := make(map[string]types.TypeID)
	for _, edge := range proposal.Mutations.Edges {
		if tablet := groups().Tablet(edge.Attr); tablet != nil && tablet.ReadOnly {
			err = errPredicateMoving
			return
		}
		if edge.Entity == 0 && bytes.Equal(edge.Value, []byte(x.Star)) {
			// We should only have one edge drop in one mutation call.
			ctx, _ := s.n.props.CtxAndTxn(proposal.Id)
			if err = s.n.Applied.WaitForMark(ctx, index-1); err != nil {
				return
			}
			tctxs := posting.Txns().Iterate(func(key []byte) bool {
				pk := x.Parse(key)
				return pk.Attr == edge.Attr
			})
			if len(tctxs) > 0 {
				go fixConflicts(tctxs)
				err = posting.ErrConflict
			} else {
				err = posting.DeletePredicate(ctx, edge.Attr)
			}
			posting.SyncMarks().Done(index)
			return
		}
		if _, ok := schemaMap[edge.Attr]; !ok {
			schemaMap[edge.Attr] = posting.TypeID(edge)
		}
	}
	if proposal.Mutations.StartTs == 0 {
		return errors.New("StartTs must be provided.")
	}
	if len(proposal.Mutations.PrimaryAttr) == 0 {
		return errors.New("Primary attribute must be provided.")
	}

	total := len(proposal.Mutations.Edges)
	s.n.props.IncRef(proposal.Id, total)
	x.ActiveMutations.Add(int64(total))
	for attr, storageType := range schemaMap {
		if _, err := schema.State().TypeOf(attr); err != nil {
			// Schema doesn't exist
			// Since committed entries are serialized, updateSchemaIfMissing is not
			// needed, In future if schema needs to be changed, it would flow through
			// raft so there won't be race conditions between read and update schema
			updateSchemaType(attr, storageType, index)
		}
	}

	m := proposal.Mutations
	pctx := s.n.props.pctx(proposal.Id)
	txn := &posting.Txn{
		StartTs:       m.StartTs,
		PrimaryAttr:   m.PrimaryAttr,
		ServesPrimary: groups().ServesTablet(m.PrimaryAttr),
	}
	pctx.txn = posting.Txns().PutOrMergeIndex(txn)
	for _, edge := range m.Edges {
		t := &task{
			rid:  index,
			pid:  proposal.Id,
			edge: edge,
		}
		if s.register(t) {
			s.tch <- t
		}
	}
	err = nil
	// Block until the above edges are applied.
	return
}

func (s *scheduler) nextTask(t *task) *task {
	s.Lock()
	defer s.Unlock()
	key := t.key()
	var nextTask *task
	tasks, ok := s.tasks[key]
	x.AssertTrue(ok)
	tasks = tasks[1:]
	if len(tasks) > 0 {
		s.tasks[key] = tasks
		nextTask = tasks[0]
	} else {
		delete(s.tasks, key)
	}
	return nextTask
}
