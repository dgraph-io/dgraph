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

// predMeta stores processing channel and closer for single predicate.
type predMeta struct {
	ch     chan *subMutation
	closer *y.Closer
}

type executor struct {
	pendingSize int64

	sync.RWMutex
	predMap map[string]*predMeta
	closer  *y.Closer
	applied *y.WaterMark
	paused  int32
	closed  bool
}

func newExecutor(applied *y.WaterMark) *executor {
	ex := &executor{
		predMap: make(map[string]*predMeta),
		closer:  y.NewCloser(0),
		applied: applied,
	}
	return ex
}

func (e *executor) processMutationCh(pmeta *predMeta) {
	defer pmeta.closer.Done()

	writer := posting.NewTxnWriter(pstore)
	for payload := range pmeta.ch {
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

func (e *executor) close() {
	e.Lock()
	defer e.Unlock()

	e.closed = true
	for _, pmeta := range e.predMap {
		close(pmeta.ch)
		pmeta.closer.Wait()
	}
}

// getChannel obtains the channel for the given pred. It must be called under e.Lock().
func (e *executor) getChannel(pred string) (ch chan *subMutation) {
	meta, ok := e.predMap[pred]
	if ok {
		return meta.ch
	}

	meta = &predMeta{
		ch:     make(chan *subMutation, 1000),
		closer: y.NewCloser(1),
	}
	e.predMap[pred] = meta
	go e.processMutationCh(meta)
	return ch
}

const (
	maxPendingEdgesSize int64 = 64 << 20
	executorAddEdges          = "executor.addEdges"
)

func uniqPredicates(schemas []*pb.SchemaUpdate) []string {
	var upreds []string
	if len(schemas) == 0 {
		return upreds
	}

	umap := make(map[string]struct{})
	for _, schema := range schemas {
		umap[schema.Predicate] = struct{}{}
	}

	for pred := range umap {
		upreds = append(upreds, pred)
	}

	return upreds
}

func (e *executor) pausePredicates(schemas ...*pb.SchemaUpdate) error {
	preds := uniqPredicates(schemas)
	if len(preds) == 0 {
		return nil
	}

	// Only one indexing can be in progress at a time. Below blocks ensures it.
	if !atomic.CompareAndSwapInt32(&e.paused, 0, 1) {
		return errors.Errorf("Error pausing predicates, cannot pause more than once at a time")
	}

	closers := make([]*y.Closer, 0, len(schemas))

	e.Lock()
	for _, pred := range preds {
		meta, ok := e.predMap[pred]
		if ok {
			// close current channel and get closer for waiting later.
			close(meta.ch)
			closers = append(closers, meta.closer)
		}

		// Create new channel and closer for predicates. We don't want to block new mutations.
		// Hence all of new mutations will be buffered inside new channel. Once someone calls
		// resumePredicates(), it will attach processing goroutines to predicate channels.
		meta = &predMeta{
			closer: y.NewCloser(0),
			ch:     make(chan *subMutation, 1000),
		}

		e.predMap[pred] = meta
	}
	e.Unlock()

	for _, closer := range closers {
		closer.Wait()
	}

	return nil
}

func (e *executor) resumePredicates(schemas ...*pb.SchemaUpdate) error {
	preds := uniqPredicates(schemas)
	if len(preds) == 0 {
		return nil
	}

	if atomic.LoadInt32(&e.paused) == 0 {
		return errors.Errorf("Error resuming predicates, nothing to resume")
	}

	e.RLock()
	defer e.RUnlock()
	for _, pred := range preds {
		meta, ok := e.predMap[pred]
		if !ok {
			continue
		}

		meta.closer.AddRunning(1)
		go e.processMutationCh(meta)
	}
	atomic.StoreInt32(&e.paused, 0)
	return nil
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

	if e.closed {
		return
	}

	for attr, payload := range payloadMap {
		e.applied.Begin(index)
		e.getChannel(attr) <- payload
	}

	atomic.AddInt64(&e.pendingSize, esize)
}
