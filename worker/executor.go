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
	"github.com/golang/glog"
	"github.com/pkg/errors"
)

type subMutation struct {
	edges   []*pb.DirectedEdge
	ctx     context.Context
	startTs uint64
	index   uint64
}

type executor struct {
	pendingSize int64

	sync.RWMutex
	predChan  map[string]chan *subMutation
	closerMap map[string]*y.Closer
	closer    *y.Closer
	applied   *y.WaterMark
	paused    int32
}

func newExecutor(applied *y.WaterMark) *executor {
	ex := &executor{
		predChan:  make(map[string]chan *subMutation),
		closerMap: make(map[string]*y.Closer),
		closer:    y.NewCloser(0),
		applied:   applied,
	}
	go ex.shutdown()
	return ex
}

func (e *executor) processMutationCh(ch chan *subMutation, closer *y.Closer) {
	defer closer.Done()

	writer := posting.NewTxnWriter(pstore)
	for payload := range ch {
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
		// TODO(Animesh): We might not need this wait.
		if err := writer.Wait(); err != nil {
			glog.Errorf("Error while waiting for writes: %v", err)
		}

		e.applied.Done(payload.index)
		atomic.AddInt64(&e.pendingSize, -esize)
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

func (e *executor) waitForClosers() {
	e.RLock()
	defer e.RUnlock()

	for _, closer := range e.closerMap {
		closer.Wait()
	}
}

// getChannelUnderLock obtains the channel for the given pred. It must be called under e.Lock().
func (e *executor) getChannelUnderLock(pred string) (ch chan *subMutation) {
	ch, ok := e.predChan[pred]
	if ok {
		return ch
	}

	e.closerMap[pred] = y.NewCloser(1)
	ch = make(chan *subMutation, 1000)
	e.predChan[pred] = ch
	go e.processMutationCh(ch, e.closerMap[pred])
	return ch
}

const (
	maxPendingEdgesSize int64 = 64 << 20
	executorAddEdges          = "executor.addEdges"
)

func (e *executor) pausePredicates(schemas ...*pb.SchemaUpdate) error {
	if atomic.LoadInt32(&e.paused) > 0 {
		return errors.Errorf("Error pausing predicates, cannot pause more than once at a time")
	}

	atomic.StoreInt32(&e.paused, 1)
	closers := make([]*y.Closer, 0, len(schemas))

	e.Lock()
	for _, schema := range schemas {
		ch, ok := e.predChan[schema.Predicate]
		if ok {
			// close current channel and get closer for waiting later.
			close(ch)
			closers = append(closers, e.closerMap[schema.Predicate])
		}

		// Create new channel and closer for predicates.
		e.closerMap[schema.Predicate] = y.NewCloser(0)
		newCh := make(chan *subMutation, 1000)
		e.predChan[schema.Predicate] = newCh
	}
	e.Unlock()

	for _, closer := range closers {
		closer.Wait()
	}

	return nil
}

func (e *executor) resumePredicates(schemas ...*pb.SchemaUpdate) error {
	if atomic.LoadInt32(&e.paused) == 0 {
		return errors.Errorf("Error resuming predicates, nothing to resume")
	}

	e.RLock()
	defer e.RUnlock()
	for _, schema := range schemas {
		ch, ok := e.predChan[schema.Predicate]
		if !ok {
			continue
		}

		closer := e.closerMap[schema.Predicate]
		closer.AddRunning(1)
		go e.processMutationCh(ch, closer)
	}
	atomic.StoreInt32(&e.paused, 0)
	return nil
}

func (e *executor) resumePredicate(pred string) {

}

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
			e.getChannelUnderLock(attr) <- payload
		}
	}

	atomic.AddInt64(&e.pendingSize, esize)
}
