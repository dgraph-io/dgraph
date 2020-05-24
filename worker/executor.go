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
	"strconv"
	"sync"

	"github.com/dgraph-io/badger/v2/y"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/ristretto/z"
	"github.com/golang/glog"
)

type subMutation struct {
	edges   []*pb.DirectedEdge
	ctx     context.Context
	startTs uint64
}

type executor struct {
	sync.RWMutex
	edgesChan []chan *subMutation
	closer    *y.Closer
}

func newExecutor() *executor {
	e := &executor{
		edgesChan: make([]chan *subMutation, 32), /* TODO: no of chans can be made configurable */
		closer:    y.NewCloser(0),
	}

	for i := 0; i < len(e.edgesChan); i++ {
		e.edgesChan[i] = make(chan *subMutation, 1000)
		e.closer.AddRunning(1)
		go e.processMutationCh(e.edgesChan[i])
	}
	go e.shutdown()
	return e
}

func (e *executor) processMutationCh(ch chan *subMutation) {
	defer e.closer.Done()

	writer := posting.NewTxnWriter(pstore)
	for payload := range ch {
		ptxn := posting.NewTxn(payload.startTs)
		for _, edge := range payload.edges {
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
	}
}

func (e *executor) shutdown() {
	<-e.closer.HasBeenClosed()
	e.Lock()
	defer e.Unlock()
	for _, ch := range e.edgesChan {
		close(ch)
	}
}

// getChannelID obtains the channel for the given edge.
func (e *executor) getChannelID(edge *pb.DirectedEdge) int {
	cid := z.MemHashString(edge.Attr+strconv.FormatUint(edge.Entity, 10)) % uint64(len(e.edgesChan))
	return int(cid)
}

func (e *executor) addEdges(ctx context.Context, startTs uint64, edges []*pb.DirectedEdge) {
	payloadMap := make(map[int]*subMutation)

	for _, edge := range edges {
		cid := e.getChannelID(edge)
		payload, ok := payloadMap[cid]
		if !ok {
			payloadMap[cid] = &subMutation{
				ctx:     ctx,
				startTs: startTs,
			}
			payload = payloadMap[cid]
		}
		payload.edges = append(payload.edges, edge)
	}

	// RLock() in case the channel gets closed from underneath us.
	e.RLock()
	defer e.RUnlock()
	select {
	case <-e.closer.HasBeenClosed():
		return
	default:
		// Closer is not closed. And we have the RLock, so sending on channel should be safe.
		for cid, payload := range payloadMap {
			e.edgesChan[cid] <- payload
		}
	}

}
