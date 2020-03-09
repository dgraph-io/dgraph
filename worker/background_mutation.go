package worker

import (
	"context"
	"sync"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/golang/glog"
)

type runMutPayload struct {
	edges   []*pb.DirectedEdge
	ctx     context.Context
	startTs uint64
}

type Executor struct {
	sync.RWMutex
	predChan      map[string]chan *runMutPayload
}

func newExecutor() (*Executor) {
	return &Executor{
		predChan: make(map[string]chan *runMutPayload),
	}
}

func (e *Executor) processMutationCh(ch chan *runMutPayload) {
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
	}
}

func (e *Executor) getChannel(pred string) (ch chan *runMutPayload) {
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
	ch = make(chan *runMutPayload, 1000)
	e.predChan[pred] = ch
	e.Unlock()
	go e.processMutationCh(ch)
	return ch
}

func (e *Executor) addEdges(ctx context.Context, StartTs uint64, edges []*pb.DirectedEdge) {
	payloadMap := make(map[string]*runMutPayload)

	for _, edge := range edges {
		payload, ok := payloadMap[edge.Attr]
		if !ok {
			payloadMap[edge.Attr] = &runMutPayload{
				ctx:     ctx,
				startTs: StartTs,
			}
			payload = payloadMap[edge.Attr]
		}
		payload.edges = append(payload.edges, edge)
	}

	for attr, payload := range payloadMap {
		e.getChannel(attr) <- payload
	}
}
