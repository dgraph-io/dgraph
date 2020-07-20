/*
 * Copyright 2016-2020 Dgraph Labs, Inc. and Contributors
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

// Package worker contains code for pb.worker communication to perform
// queries and mutations.
package worker

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/dgraph-io/badger/v2/y"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
)

type subMutation struct {
	edges   []*pb.DirectedEdge
	ctx     context.Context
	startTs uint64
	index   uint64
}

type executor struct {
	pendingSize int64
	smCount     int64 // Stores count for active sub mutations.

	sync.RWMutex
	predChan   map[string]chan *subMutation
	workerChan map[string]chan *mutation
	closer     *y.Closer
	applied    *y.WaterMark
}

func newExecutor(applied *y.WaterMark) *executor {
	ex := &executor{
		predChan:   make(map[string]chan *subMutation),
		closer:     y.NewCloser(0),
		applied:    applied,
		workerChan: make(map[string]chan *mutation),
	}

	go ex.shutdown()
	return ex
}

func generateConflictKeys(p *subMutation) []uint64 {
	keys := make([]uint64, 0)

	for _, edge := range p.edges {
		key := x.DataKey(edge.Attr, edge.Entity)
		pk, err := x.Parse(key)
		if err != nil {
			continue
		}

		keys = append(keys, posting.GetConflictKeys(pk, key, edge))
	}

	return keys
}

type mutation struct {
	m     *subMutation
	keys  []uint64
	inDeg int

	outEdges []*mutation

	graph *graph
}

type graph struct {
	sync.RWMutex
	conflicts map[uint64][]*mutation
}

func newGraph() *graph {
	return &graph{conflicts: make(map[uint64][]*mutation)}
}

func (e *executor) worker(ch chan *mutation) {
	writer := posting.NewTxnWriter(pstore)
	for mutation := range ch {
		payload := mutation.m

		var esize int64
		ptxn := posting.NewTxn(payload.startTs)
		for _, edge := range payload.edges {
			esize += int64(edge.Size())
			for {
				err := runMutation(payload.ctx, edge, ptxn)
				if err == nil {
					break
				} else if err != posting.ErrRetry {
					glog.Errorf("Error while mutating: %v", err)
					break
				}
			}
		}
		ptxn.Update()
		if err := ptxn.CommitToDisk(writer, payload.startTs); err != nil {
			glog.Errorf("Error while commiting to disk: %v", err)
		}

		if err := writer.Wait(); err != nil {
			glog.Errorf("Error while waiting for writes: %v", err)
		}

		e.applied.Done(payload.index)
		atomic.AddInt64(&e.pendingSize, -esize)
		atomic.AddInt64(&e.smCount, -1)

		mutation.graph.Lock()
		for _, c := range mutation.keys {
			i := 0
			arr := mutation.graph.conflicts[c]

			for _, x := range arr {
				if x.m.startTs != mutation.m.startTs {
					arr[i] = x
					i++
				}
			}

			if i == 0 {
				delete(mutation.graph.conflicts, c)
			} else {
				mutation.graph.conflicts[c] = arr[:i]
			}
		}
		mutation.graph.Unlock()

		for _, dependent := range mutation.outEdges {
			dependent.inDeg -= 1
			if dependent.inDeg == 0 {
				ch <- dependent
			}
		}

	}
}

func (e *executor) processMutationCh(ch chan *subMutation, workerCh chan *mutation) {
	defer e.closer.Done()

	g := newGraph()

	for payload := range ch {
		conflicts := generateConflictKeys(payload)
		m := mutation{m: payload, keys: conflicts, outEdges: make([]*mutation, 0), graph: g}

		g.Lock()
		for _, c := range conflicts {
			l, ok := g.conflicts[c]
			if !ok {
				l = append(l, &m)
				continue
			}

			m.inDeg += len(l)
			for _, dependent := range l {
				dependent.outEdges = append(dependent.outEdges, &m)
			}

			l = append(l, &m)
		}
		g.Unlock()

		if m.inDeg == 0 {
			workerCh <- &m
		}
	}
}

func (e *executor) shutdown() {
	<-e.closer.HasBeenClosed()
	e.RLock()
	defer e.RUnlock()
	for _, ch := range e.predChan {
		close(ch)
	}
}

// getChannel obtains the channel for the given pred. It must be called under e.Lock().
func (e *executor) getChannel(pred string) (ch chan *subMutation) {
	ch, ok := e.predChan[pred]
	if ok {
		return ch
	}
	ch = make(chan *subMutation, 1000)
	workerCh := make(chan *mutation, 1000)
	e.predChan[pred] = ch
	e.closer.AddRunning(1)
	go e.processMutationCh(ch, workerCh)
	for i := 0; i < 2; i++ {
		go e.worker(workerCh)
	}
	return ch
}

const (
	maxPendingEdgesSize int64 = 64 << 20
	executorAddEdges          = "executor.addEdges"
)

func (e *executor) addEdges(ctx context.Context, proposal *pb.Proposal) {
	rampMeter(&e.pendingSize, maxPendingEdgesSize, executorAddEdges)

	index := proposal.Index
	startTs := proposal.Mutations.StartTs
	edges := proposal.Mutations.Edges

	payloadMap := make(map[string]*subMutation)
	var esize int64
	for _, edge := range edges {
		payload, ok := payloadMap[edge.Attr]
		if !ok {
			payloadMap[edge.Attr] = &subMutation{
				ctx:     ctx,
				startTs: startTs,
				index:   index,
			}
			payload = payloadMap[edge.Attr]
		}
		payload.edges = append(payload.edges, edge)
		esize += int64(edge.Size())
	}

	// Lock() in case the channel gets closed from underneath us.
	e.Lock()
	defer e.Unlock()
	select {
	case <-e.closer.HasBeenClosed():
		return
	default:
		// Closer is not closed. And we have the Lock, so sending on channel should be safe.
		for attr, payload := range payloadMap {
			e.applied.Begin(index)
			atomic.AddInt64(&e.smCount, 1)
			e.getChannel(attr) <- payload
		}
	}

	atomic.AddInt64(&e.pendingSize, esize)
}

// waitForActiveMutations waits for all the mutations (currently active) to finish. This function
// should be called before running any schema mutation.
func (e *executor) waitForActiveMutations() {
	glog.Infoln("executor: wait for active mutation to finish")
	rampMeter(&e.smCount, 0, "waiting on active mutations to finish")
}
