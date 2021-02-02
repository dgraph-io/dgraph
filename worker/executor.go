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
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/dgraph-io/badger/v3/y"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
	"github.com/dgryski/go-farm"
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
	predChan map[string]chan *subMutation
	closer   *z.Closer
	applied  *y.WaterMark
	throttle *y.Throttle
}

func newExecutor(applied *y.WaterMark, conc int) *executor {
	ex := &executor{
		predChan: make(map[string]chan *subMutation),
		closer:   z.NewCloser(0),
		applied:  applied,
		throttle: y.NewThrottle(conc), // Run conc threads mutations at a time.
	}

	go ex.shutdown()
	return ex
}

func generateTokenKeys(nq *pb.DirectedEdge, tokenizers []tok.Tokenizer) ([]uint64, error) {
	keys := make([]uint64, 0, len(tokenizers))
	errs := make([]string, 0)
	for _, token := range tokenizers {
		storageVal := types.Val{
			Tid:   types.TypeID(nq.GetValueType()),
			Value: nq.GetValue(),
		}

		schemaVal, err := types.Convert(storageVal, types.TypeID(nq.GetValueType()))
		if err != nil {
			errs = append(errs, err.Error())
		}
		toks, err := tok.BuildTokens(schemaVal.Value, tok.GetTokenizerForLang(token,
			nq.Lang))
		if err != nil {
			errs = append(errs, err.Error())
		}

		for _, t := range toks {
			keys = append(keys, farm.Fingerprint64(x.IndexKey(nq.Attr, t)))
		}
	}

	if len(errs) > 0 {
		return keys, fmt.Errorf(strings.Join(errs, "\n"))
	}
	return keys, nil
}

func generateConflictKeys(ctx context.Context, p *subMutation) map[uint64]struct{} {
	keys := make(map[uint64]struct{})

	for _, edge := range p.edges {
		key := x.DataKey(edge.Attr, edge.Entity)
		pk, err := x.Parse(key)
		if err != nil {
			glog.V(2).Info("Error in generating conflict keys", err)
			continue
		}

		keys[posting.GetConflictKey(pk, key, edge)] = struct{}{}
		stt, _ := schema.State().Get(ctx, edge.Attr)
		tokenizers := schema.State().Tokenizer(ctx, edge.Attr)
		isReverse := schema.State().IsReversed(ctx, edge.Attr)

		if stt.Count || isReverse {
			keys[0] = struct{}{}
		}

		tokens, err := generateTokenKeys(edge, tokenizers)
		for _, token := range tokens {
			keys[token] = struct{}{}
		}
		if err != nil {
			glog.V(2).Info("Error in generating token keys", err)
		}
	}

	return keys
}

type mutation struct {
	inDeg              int64
	sm                 *subMutation
	conflictKeys       map[uint64]struct{}
	dependentMutations map[uint64]*mutation
	graph              *graph
}

type graph struct {
	sync.RWMutex
	conflicts map[uint64][]*mutation
}

func newGraph() *graph {
	return &graph{conflicts: make(map[uint64][]*mutation)}
}

func (e *executor) worker(mut *mutation) {
	writer := posting.NewTxnWriter(pstore)
	payload := mut.sm

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

	e.throttle.Done(nil)

	mut.graph.Lock()
	defer mut.graph.Unlock()

	// Decrease inDeg of dependents. If this mutation unblocks them, queue them.
	for _, dependent := range mut.dependentMutations {
		if atomic.AddInt64(&dependent.inDeg, -1) == 0 {
			x.Check(e.throttle.Do())
			go e.worker(dependent)
		}
	}

	for key := range mut.conflictKeys {
		// remove the transaction from each conflict key's entry in the conflict map.
		i := 0
		arr := mut.graph.conflicts[key]
		for _, x := range arr {
			if x.sm.startTs != mut.sm.startTs {
				arr[i] = x
				i++
			}
		}

		if i == 0 {
			// if conflict key is now empty, delete it.
			delete(mut.graph.conflicts, key)
		} else {
			mut.graph.conflicts[key] = arr[:i]
		}
	}
}

func (e *executor) processMutationCh(ctx context.Context, ch chan *subMutation) {
	defer e.closer.Done()

	g := newGraph()

	for payload := range ch {
		conflicts := generateConflictKeys(ctx, payload)
		m := &mutation{
			sm:                 payload,
			conflictKeys:       conflicts,
			dependentMutations: make(map[uint64]*mutation),
			graph:              g,
		}

		isDependent := false
		g.Lock()
		for c := range conflicts {
			l, ok := g.conflicts[c]
			if !ok {
				g.conflicts[c] = []*mutation{m}
				continue
			}

			for _, dependent := range l {
				_, ok := dependent.dependentMutations[m.sm.startTs]
				if !ok {
					isDependent = true
					atomic.AddInt64(&m.inDeg, 1)
					dependent.dependentMutations[m.sm.startTs] = m
				}
			}

			l = append(l, m)
			g.conflicts[c] = l
		}
		g.Unlock()

		// If this mutation doesn't depend on any other mutation then process it right now.
		// Otherwise, don't process it. It will be called for processing when the last mutation on
		// which it depends is completed.
		if !isDependent {
			x.Check(e.throttle.Do())
			go e.worker(m)
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
func (e *executor) getChannel(ctx context.Context, pred string) (ch chan *subMutation) {
	ch, ok := e.predChan[pred]
	if ok {
		return ch
	}
	ch = make(chan *subMutation, 1000)
	e.predChan[pred] = ch
	e.closer.AddRunning(1)
	go e.processMutationCh(ctx, ch)
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
			e.getChannel(ctx, attr) <- payload
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
