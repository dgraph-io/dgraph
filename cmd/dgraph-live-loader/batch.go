package main

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/dgraph/xidmap"
)

// batchMutationOptions sets the clients batch mode to Pending number of buffers each of Size.
// Running counters of number of rdfs processed, total time and mutations per second are printed
// if PrintCounters is set true.  See Counter.
type batchMutationOptions struct {
	Size          int
	Pending       int
	PrintCounters bool
	MaxRetries    uint32
	// User could pass a context so that we can stop retrying requests once context is done
	Ctx context.Context
}

var DefaultOptions = BatchMutationOptions{
	Size:          100,
	Pending:       100,
	PrintCounters: false,
	MaxRetries:    math.MaxUint32,
}

type uidProvider struct {
	dc  protos.DgraphClient
	ctx context.Context
}

// loader is the data structure held by the user program for all interactions with the Dgraph
// server.  After making grpc connection a new Dgraph is created by function NewDgraphClient.
type loader struct {
	opts batchMutationOptions

	schema chan protos.SchemaUpdate
	dc     []protos.DgraphClient
	alloc  *xidmap.XidMap
	ticker *time.Ticker
	kv     *badger.KV

	// Miscellaneous information to print counters.
	// Num of RDF's sent
	rdfs uint64
	// Num of mutations sent
	mutations uint64
	// To get time elapsed.
	start time.Time

	// Map of filename to x.Watermark. Used for checkpointing.
	marks            syncMarks
	reqs             chan *Req
	checkpointTicker *time.Ticker // Used to write checkpoints periodically.
	che              chan error
	// In case of max retries exceeded, we set this to 1.
	retriesExceeded int32
}

func (p *uidProvider) ReserveUidRange() (start, end uint64, err error) {
	factor := time.Second
	for {
		assignedIds, err := p.dc.AssignUids(context.Background(), &protos.Num{Val: 1000})
		if err == nil {
			return assignedIds.StartId, assignedIds.EndId, nil
		}
		x.Printf("Error while getting lease %v\n", err)
		select {
		case <-time.After(factor):
		case <-p.ctx.Done():
			return 0, 0, p.ctx.Err()
		}
		if factor < 256*time.Second {
			factor = factor * 2
		}
	}
}

// Counter keeps a track of various parameters about a batch mutation. Running totals are printed
// if BatchMutationOptions PrintCounters is set to true.
type Counter struct {
	// Number of RDF's processed by server.
	Rdfs uint64
	// Number of mutations processed by the server.
	Mutations uint64
	// Time elapsed sinze the batch started.
	Elapsed time.Duration
}

func (l *loader) request(req *Req) error {
	counter := atomic.AddUint64(&d.mutations, 1)
	factor := time.Second
	var retries uint32
RETRY:
	select {
	case <-d.opts.Ctx.Done():
		return d.opts.Ctx.Err()
	default:
	}
	_, err := d.dc[rand.Intn(len(d.dc))].Run(context.Background(), &req.gr)
	if err != nil {
		errString := err.Error()
		// Irrecoverable
		if strings.Contains(errString, "x509") || grpc.Code(err) == codes.Internal {
			return err
		}
		if !strings.Contains(errString, "Temporary Error") {
			fmt.Printf("Retrying req: %d. Error: %v\n", counter, errString)
		}
		time.Sleep(factor)
		if factor < 256*time.Second {
			factor = factor * 2
		}
		if retries >= d.opts.MaxRetries {
			atomic.CompareAndSwapInt32(&d.retriesExceeded, 0, 1)
			return ErrMaxTries
		}
		retries++
		goto RETRY
	}

	// Mark watermarks as done.
	if req.line != 0 && req.mark != nil {
		req.mark.Done(req.line)
		req.markWg.Done()
	}
	atomic.AddUint64(&d.rdfs, uint64(req.Size()))
	return nil
}

// makeRequests can receive requests from batchNquads or directly from BatchSetWithMark.
// It doesn't need to batch the requests anymore. Batching is already done for it by the
// caller functions.
func (l *loader) makeRequests() {
	for req := range d.reqs {
		if atomic.LoadInt32(&d.retriesExceeded) == 1 {
			d.che <- ErrMaxTries
			return
		}

		if err := d.request(req); err != nil {
			d.che <- err
			return
		}
	}
	d.che <- nil
}

// Used to batch the nquads into Req of size given by user and send to d.reqs channel.
func (l *loader) batchNquads() {
	req := new(Req)
	for n := range d.nquads {
		if atomic.LoadInt32(&d.retriesExceeded) == 1 {
			d.che <- ErrMaxTries
			return
		}

		if n.op == SET {
			req.Set(n.e)
		} else if n.op == DEL {
			req.Delete(n.e)
		}
		if req.Size() == d.opts.Size {
			d.reqs <- req
			req = new(Req)
		}
	}

	if req.Size() > 0 {
		d.reqs <- req
	}
	close(d.reqs)
	d.che <- nil
}

func (l *loader) printCounters() {
	l.ticker = time.NewTicker(2 * time.Second)
	start := time.Now()

	for range l.ticker.C {
		counter := l.Counter()
		rate := float64(counter.Rdfs) / counter.Elapsed.Seconds()
		elapsed := ((time.Since(start) / time.Second) * time.Second).String()
		fmt.Printf("[Request: %6d] Total RDFs done: %8d RDFs per second: %7.0f Time Elapsed: %v \r",
			counter.Mutations, counter.Rdfs, rate, elapsed)

	}
}

// TODO - Temporarily break checkpointing.
// BatchSetWithMark takes a Req which has a batch of edges. It accepts a file to which the edges
// belong and also the line number of the last line that the batch contains. This is used by the
// dgraph-live-loader to do checkpointing so that in case the loader crashes, we can skip the lines
// which the server has already processed. Most users would only need BatchSet which does the
// batching automatically.
// func (l *loader) BatchSetWithMark(r *Req, file string, line uint64) error {
// 	sm := d.marks[file]
// 	if sm.mark != nil && line != 0 {
// 		r.mark = sm.mark
// 		r.line = line
// 		r.markWg = sm.wg
// 		sm.mark.Begin(line)
// 		sm.wg.Add(1)
// 	}
//
// L:
// 	for {
// 		select {
// 		case <-d.opts.Ctx.Done():
// 			return d.opts.Ctx.Err()
// 		case d.reqs <- r:
// 			break L
// 		default:
// 			if atomic.LoadInt32(&d.retriesExceeded) == 1 {
// 				return ErrMaxTries
// 			}
// 			time.Sleep(5 * time.Millisecond)
// 		}
// 	}
// 	return nil
// }
