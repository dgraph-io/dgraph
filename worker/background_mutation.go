/*
 * Copyright 2016-2018 Dgraph Labs, Inc. and Contributors
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

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/golang/glog"
)

type subMutation struct {
	edges   []*pb.DirectedEdge
	ctx     context.Context
	startTs uint64
}

type Executor struct {
	sync.RWMutex
	predChan map[string]chan *subMutation
}

func newExecutor() *Executor {
	return &Executor{
		predChan: make(map[string]chan *subMutation),
	}
}

func (e *Executor) processMutationCh(ch chan *subMutation) {
	writer := posting.NewTxnWriter(pstore)
	for payload := range ch {
		ptxn := posting.NewTxn(payload.startTs)
		for _, edge := range payload.edges {
			for {
				err := runMutation(payload.ctx, edge, ptxn)
				if err == nil {
					break
				}
				if err != posting.ErrRetry {
					glog.Errorf("Error while mutating: %+v", err)
					break
				}
			}
		}
		ptxn.Update()
		if err := ptxn.CommitToDisk(writer, payload.startTs); err != nil {
			glog.Errorf("Error while commiting to disk: %+v", err)
		}
		// TODO(Animesh): We might not need this wait.
		writer.Wait()
	}
}

func (e *Executor) getChannel(pred string) (ch chan *subMutation) {
	e.RLock()
	ch, ok := e.predChan[pred]
	e.RUnlock()
	if ok {
		return ch
	}

	// Create a new channel for `pred`.
	e.Lock()
	ch, ok = e.predChan[pred]
	if ok {
		e.Unlock()
		return ch
	}
	ch = make(chan *subMutation, 1000)
	e.predChan[pred] = ch
	e.Unlock()
	go e.processMutationCh(ch)
	return ch
}

func (e *Executor) addEdges(ctx context.Context, startTs uint64, edges []*pb.DirectedEdge) {
	payloadMap := make(map[string]*subMutation)

	for _, edge := range edges {
		payload, ok := payloadMap[edge.Attr]
		if !ok {
			payloadMap[edge.Attr] = &subMutation{
				ctx:     ctx,
				startTs: startTs,
			}
			payload = payloadMap[edge.Attr]
		}
		payload.edges = append(payload.edges, edge)
	}

	for attr, payload := range payloadMap {
		e.getChannel(attr) <- payload
	}
}
