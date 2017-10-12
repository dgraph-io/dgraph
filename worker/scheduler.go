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
	"fmt"
	"sync"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
	farm "github.com/dgryski/go-farm"
)

type task struct {
	rid    uint64 // raft index corresponding to the task
	pid    uint32 // proposal id corresponding to the task
	edge   *protos.DirectedEdge
	upsert *protos.Query
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
			n.props.Done(nextTask.pid, err)
			x.ActiveMutations.Add(-1)
			nextTask = s.nextTask(nextTask)
		}
	}
}

func (t *task) key() uint32 {
	if t.upsert != nil && t.upsert.Attr == t.edge.Attr {
		// Serialize upserts by predicate.
		return farm.Fingerprint32([]byte(t.edge.Attr))
	}

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
func (s *scheduler) schedule(proposal *protos.Proposal, index uint64) error {
	if proposal.Mutations.DropAll {
		if err := s.n.waitForSyncMark(s.n.ctx, index-1); err != nil {
			s.n.props.Done(proposal.Id, err)
			return err
		}
		schema.State().DeleteAll()
		if err := posting.DeleteAll(); err != nil {
			s.n.props.Done(proposal.Id, err)
			return err
		}
		s.n.props.Done(proposal.Id, nil)
		return nil
	}

	// ensures that index is not mark completed until all tasks
	// are submitted to scheduler
	total := len(proposal.Mutations.Edges)
	for _, supdate := range proposal.Mutations.Schema {
		// This is neceassry to ensure that there is no race between when we start reading
		// from badger and new mutation getting commited via raft and getting applied.
		// Before Moving the predicate we would flush all and wait for watermark to catch up
		// but there might be some proposals which got proposed but not comitted yet.
		// It's ok to reject the proposal here and same would happen on all nodes because we
		// would have proposed membershipstate, and all nodes would have the proposed state
		// or some state after that before reaching here.
		if tablet := groups().Tablet(supdate.Predicate); tablet != nil && tablet.ReadOnly {
			s.n.props.Done(proposal.Id, errPredicateMoving)
			return errPredicateMoving
		}
		if err := s.n.processSchemaMutations(proposal.Id, index, supdate); err != nil {
			s.n.props.Done(proposal.Id, err)
			return err
		}
	}
	if total == 0 {
		s.n.props.Done(proposal.Id, nil)
		return nil
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
			s.n.props.Done(proposal.Id, errPredicateMoving)
			return errPredicateMoving
		}
		if _, ok := schemaMap[edge.Attr]; !ok {
			schemaMap[edge.Attr] = posting.TypeID(edge)
		}
	}
	if proposal.Mutations.Upsert != nil {
		attr := proposal.Mutations.Upsert.Attr
		if tablet := groups().Tablet(attr); tablet != nil && tablet.ReadOnly {
			s.n.props.Done(proposal.Id, errPredicateMoving)
			return errPredicateMoving
		}
	}

	s.n.props.IncRef(proposal.Id, total)
	x.ActiveMutations.Add(int64(total))
	for attr, storageType := range schemaMap {
		if _, err := schema.State().TypeOf(attr); err != nil {
			// Schema doesn't exist
			// Since committed entries are serialized, updateSchemaIfMissing is not
			// needed, In future if schema needs to be changed, it would flow through
			// raft so there won't be race conditions between read and update schema
			updateSchemaType(attr, storageType, index, s.n.gid)
		}
	}

	for _, edge := range proposal.Mutations.Edges {
		t := &task{
			rid:    index,
			pid:    proposal.Id,
			edge:   edge,
			upsert: proposal.Mutations.Upsert,
		}
		if s.register(t) {
			s.tch <- t
		}
	}
	s.n.props.Done(proposal.Id, nil)
	return nil
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
