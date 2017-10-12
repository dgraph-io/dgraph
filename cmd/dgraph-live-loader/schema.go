package main

import (
	"sync/atomic"
	"time"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/protos"
)

func (l *loader) makeSchemaRequests() {
	req := new(client.Req)
LOOP:
	for {
		select {
		case s, ok := <-l.schema:
			if !ok {
				break LOOP
			}
			req.AddSchema(s)
		default:
			if atomic.LoadInt32(&l.retriesExceeded) == 1 {
				l.che <- ErrMaxTries
				return
			}
			start := time.Now()
			if req.Size() > 0 {
				l.request(req)
				req = new(client.Req)
			}
			elapsedMillis := time.Since(start).Seconds() * 1e3
			if elapsedMillis < 10 {
				time.Sleep(time.Duration(int64(10-elapsedMillis)) * time.Millisecond)
			}
		}
	}

	if req.Size() > 0 {
		l.request(req)
	}
	l.che <- nil
}

// AddSchema adds the given schema mutation to the batch of schema mutations.  If the schema
// mutation applies an index to a UID edge, or if it adds reverse to a scalar edge, then the
// mutation is not added to the batch and an error is returned. Once added, the client will
// apply the schema mutation when it is ready to flush its buffers.
func (l *loader) AddSchema(s protos.SchemaUpdate) error {
	if err := l.dc.CheckSchema(s); err != nil {
		return err
	}
	l.schema <- s
	return nil
}
