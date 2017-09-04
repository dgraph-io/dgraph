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
			err := s.n.processMutation(nextTask.pid, nextTask.rid, nextTask.edge)
			n.props.Done(nextTask.pid, err)
			x.ActiveMutations.Add(-1)
			nextTask = s.nextTask(nextTask)
		}
	}
}

func taskKey(attribute string, uid uint64) uint32 {
	key := fmt.Sprintf("%s|%d", attribute, uid)
	return farm.Fingerprint32([]byte(key))
}

func (s *scheduler) register(t *task) bool {
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

func (s *scheduler) schedule(proposal *protos.Proposal, index uint64) error {
	// ensures that index is not mark completed until all tasks
	// are submitted to scheduler
	total := len(proposal.Mutations.Edges)
	s.n.props.IncRef(proposal.Id, index, 1+total)
	x.ActiveMutations.Add(int64(total))
	for _, supdate := range proposal.Mutations.Schema {
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
		t := &task{
			rid:  index,
			pid:  proposal.Id,
			edge: edge,
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
