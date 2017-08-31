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
	ptch  chan *task // priority task channel
	// If we push proposals to a separate list while schema mutation is
	// going on then we can't have the ordering guarantee even if we
	// check in priority buffer first.
	// For now doing a simple design where we block the scheduler loop
	// if a mutation comes which is using the predicate. Dgraph would be
	// under heavy load while doing schema mutations so it should be ok to block.
	schemaMap     map[string]chan struct{}
	pendingSchema chan struct{}

	n *node
}

func (s *scheduler) init(n *node) {
	s.n = n
	s.tasks = make(map[uint32][]*task)
	s.tch = make(chan *task, 10000)
	s.ptch = make(chan *task, 10000)
	s.schemaMap = make(map[string]chan struct{})
	s.pendingSchema = make(chan struct{}, 10)
	for i := 0; i < 1000; i++ {
		go s.processTasks()
	}
}

func (s *scheduler) processTasks() {
	for {
		// Check if something is there in priority channel
		select {
		case t := <-s.ptch:
			s.executeMutation(t)
		default:
		}
		select {
		case t := <-s.ptch:
			s.executeMutation(t)
		case t := <-s.tch:
			s.executeMutation(t)
		}
	}
}

func (s *scheduler) executeMutation(t *task) {
	err := s.n.processMutation(t.pid, t.rid, t.edge)
	s.n.props.Done(t.pid, err)
	s.n.applied.Done(t.rid)
	posting.SyncMarkFor(s.n.gid).Done(t.rid)
	x.ActiveMutations.Add(-1)
	if nextTask := s.nextTask(t); nextTask != nil {
		s.ptch <- nextTask
	}
}

func taskKey(attribute string, uid uint64) uint32 {
	key := fmt.Sprintf("%s|%d", attribute, uid)
	return farm.Fingerprint32([]byte(key))
}

func (s *scheduler) canSchedule(t *task) bool {
	s.Lock()
	defer s.Unlock()
	key := taskKey(t.edge.Attr, t.edge.Entity)
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

func (s *scheduler) markSchemaDone(predicate string) {
	s.Lock()
	defer s.Unlock()
	ch, ok := s.schemaMap[predicate]
	x.AssertTrue(ok)
	ch <- struct{}{}
	close(ch)
}

func (s *scheduler) waitForSchema(predicate string) {
	s.Lock()
	s.Unlock()
	ch, ok := s.schemaMap[predicate]
	if !ok {
		return
	}
	<-ch
}

func (s *scheduler) schedule(proposal *protos.Proposal, index uint64) {
	total := len(proposal.Mutations.Edges) + len(proposal.Mutations.Schema)
	if total > 0 {
		s.n.props.IncRef(proposal.Id, total)
		x.ActiveMutations.Add(int64(total))
		s.n.applied.BeginWithCount(index, total)
		posting.SyncMarkFor(s.n.gid).BeginWithCount(index, total)
	}
	for _, supdate := range proposal.Mutations.Schema {
		s.waitForSchema(supdate.Predicate)
		s.scheduleSchema(proposal.Id, index, supdate)
	}

	// Scheduler tracks tasks at subject, predicate level, so doing
	// schema stuff here simplies the design and we needn't worrying about
	// serializing the mutations per predicate or schema mutations
	// We derive the schema here if it's not present
	// Since raft committed logs are serialized, we can derive
	// schema here without any locking
	// stores a map of predicate and type of first mutation for each predicate
	schemaMap := make(map[string]types.TypeID)
	for _, edge := range proposal.Mutations.Edges {
		if _, ok := schemaMap[edge.Attr]; !ok {
			schemaMap[edge.Attr] = posting.TypeID(edge)
		}
	}

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
		s.waitForSchema(edge.Attr)
		s.scheduleMutation(&task{
			rid:  index,
			pid:  proposal.Id,
			edge: edge,
		})
	}
}

func (s *scheduler) scheduleSchema(pid uint32, rid uint64, supdate *protos.SchemaUpdate) {
	s.Lock()
	s.schemaMap[supdate.Predicate] = make(chan struct{}, 1)
	s.Unlock()

	s.pendingSchema <- struct{}{}
	go func() {
		err := s.n.processSchemaMutations(pid, rid, supdate)
		s.n.props.Done(pid, err)
		s.markSchemaDone(supdate.Predicate)
		s.n.applied.Done(rid)
		posting.SyncMarkFor(s.n.gid).Done(rid)
		<-s.pendingSchema
	}()
}

func (s *scheduler) scheduleMutation(t *task) {
	if s.canSchedule(t) {
		s.tch <- t
	}
}

func (s *scheduler) nextTask(t *task) *task {
	s.Lock()
	defer s.Unlock()
	key := taskKey(t.edge.Attr, t.edge.Entity)
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
