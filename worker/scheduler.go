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
	tasks   map[uint32][]*task
	pending chan struct{}

	// If we push proposals to a separate list while schema mutation is
	// going on then we can't have the ordering guarantee even if we
	// check in priority buffer first.
	// For now doing a simple design where we block the scheduler loop
	// if a mutation comes which is using the predicate. Dgraph would be
	// under heavy load while doing schema mutations so it should be ok to block.
	schemaMap     map[string]chan struct{}
	pendingSchema chan struct{}
}

func (s *scheduler) init() {
	s.tasks = make(map[uint32][]*task)
	s.pending = make(chan struct{}, 100000)
	s.schemaMap = make(map[string]chan struct{})
	s.pendingSchema = make(chan struct{}, 10)
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

func (s *scheduler) submitSchemaUpdate(pid uint32, rid uint64,
	supdate *protos.SchemaUpdate, n *node) {
	s.waitForSchema(supdate.Predicate)
	s.Lock()
	s.schemaMap[supdate.Predicate] = make(chan struct{}, 1)
	s.Unlock()

	s.pendingSchema <- struct{}{}
	go func() {
		err := n.processSchemaMutations(pid, rid, supdate)
		n.props.Done(pid, err)
		s.markSchemaDone(supdate.Predicate)
		n.applied.Done(rid)
		posting.SyncMarkFor(n.gid).Done(rid)
		<-s.pendingSchema
	}()
}

func (s *scheduler) submit(t *task, n *node) {
	s.waitForSchema(t.edge.Attr)
	// used to throttle number of pending tasks
	s.pending <- struct{}{}

	if s.canSchedule(t) {
		go func() {
			n.process(t)
		}()
	}
}

func (s *scheduler) nextTask(t *task, err error, n *node) *task {
	n.props.Done(t.pid, err)
	n.applied.Done(t.rid)
	posting.SyncMarkFor(n.gid).Done(t.rid)

	key := taskKey(t.edge.Attr, t.edge.Entity)
	var nextTask *task
	s.Lock()
	tasks, ok := s.tasks[key]
	x.AssertTrue(ok)
	tasks = tasks[1:]
	if len(tasks) > 0 {
		s.tasks[key] = tasks
		nextTask = tasks[0]
	} else {
		delete(s.tasks, key)
	}
	s.Unlock()

	x.ActiveMutations.Add(-1)
	<-s.pending
	return nextTask
}
