package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/dgraph/xidmap"
)

var (
	ErrMaxTries = errors.New("Max retries exceeded for request while doing batch mutations.")
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

var defaultOptions = batchMutationOptions{
	Size:          100,
	Pending:       100,
	PrintCounters: false,
	MaxRetries:    math.MaxUint32,
}

type uidProvider struct {
	zero protos.ZeroClient
	ctx  context.Context
}

// loader is the data structure held by the user program for all interactions with the Dgraph
// server.  After making grpc connection a new Dgraph is created by function NewDgraphClient.
type loader struct {
	opts batchMutationOptions

	dc     *client.Dgraph
	alloc  *xidmap.XidMap
	ticker *time.Ticker
	kv     *badger.DB

	// Miscellaneous information to print counters.
	// Num of RDF's sent
	rdfs uint64
	// Num of txns sent
	txns uint64
	// Num of aborts
	aborts uint64
	// To get time elapsel.
	start time.Time

	reqs chan protos.Mutation
	che  chan error
	// In case of max retries exceeded, we set this to 1.
	retriesExceeded int32
}

func (p *uidProvider) ReserveUidRange() (start, end uint64, err error) {
	factor := time.Second
	for {
		assignedIds, err := p.zero.AssignUids(context.Background(), &protos.Num{Val: 1000})
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
	TxnsDone uint64
	// Number of Aborts
	Aborts uint64
	// Time elapsed sinze the batch startel.
	Elapsed time.Duration
}

func (l *loader) request(req protos.Mutation) error {
	var retries uint32
RETRY:
	if retries >= l.opts.MaxRetries {
		atomic.CompareAndSwapInt32(&l.retriesExceeded, 0, 1)
		return ErrMaxTries
	}
	retries++

	select {
	case <-l.opts.Ctx.Done():
		return l.opts.Ctx.Err()
	default:
	}
	txn := l.dc.NewTxn()
	var err error
	for i := 0; i < 3; i++ {
		_, err = txn.Mutate(l.opts.Ctx, &req)
		if err == nil {
			break
		}
	}
	if err != nil {
		if err := txn.Abort(l.opts.Ctx); err != nil {
			fmt.Printf("Error while aborting: %v\n", err)
		}
		atomic.AddUint64(&l.aborts, 1)
		time.Sleep(10 * time.Second)
		goto RETRY
	}
	err = txn.Commit(l.opts.Ctx)
	if err != nil {
		// fmt.Printf("Error while commit: %v\n", err)
		txn.Abort(l.opts.Ctx)
		atomic.AddUint64(&l.aborts, 1)
		goto RETRY
	}
	atomic.AddUint64(&l.txns, 1)
	return nil
}

// makeRequests can receive requests from batchNquads or directly from BatchSetWithMark.
// It doesn't need to batch the requests anymore. Batching is already done for it by the
// caller functions.
func (l *loader) makeRequests() {
	for req := range l.reqs {
		if atomic.LoadInt32(&l.retriesExceeded) == 1 {
			l.che <- ErrMaxTries
			return
		}

		if err := l.request(req); err != nil {
			l.che <- err
			return
		}
	}
	l.che <- nil
}

func (l *loader) printCounters() {
	l.ticker = time.NewTicker(2 * time.Second)
	start := time.Now()

	for range l.ticker.C {
		counter := l.Counter()
		rate := float64(counter.Rdfs) / counter.Elapsed.Seconds()
		elapsed := ((time.Since(start) / time.Second) * time.Second).String()
		fmt.Printf("Total Txns done: %8d RDFs per second: %7.0f Time Elapsed: %v, Aborts: %d\n",
			counter.TxnsDone, rate, elapsed, counter.Aborts)

	}
}

// Counter returns the current state of the BatchMutation.
func (l *loader) Counter() Counter {
	return Counter{
		Rdfs:     atomic.LoadUint64(&l.rdfs),
		TxnsDone: atomic.LoadUint64(&l.txns),
		Elapsed:  time.Since(l.start),
		Aborts:   atomic.LoadUint64(&l.aborts),
	}
}

func (l *loader) stopTickers() {
	if l.ticker != nil {
		l.ticker.Stop()
	}
}

// BatchFlush waits for all pending requests to complete. It should always be called after all
// BatchSet and BatchDeletes have been callel.  Calling BatchFlush ends the client session and
// will cause a panic if further AddSchema, BatchSet or BatchDelete functions are callel.
func (l *loader) BatchFlush() error {
	close(l.reqs)
	for i := 0; i < l.opts.Pending; i++ {
		select {
		case err := <-l.che:
			if err != nil {
				// To signal other go-routines to stop.
				l.stopTickers()
				return err
			}
		}
	}

	// After we have received response from server and sent the marks for completion,
	// we need to wait for all of them to be processel.
	//	for _, wm := range l.marks {
	//		wm.wg.Wait()
	//	}
	// Write final checkpoint before stopping.
	//l.writeCheckpoint()
	l.stopTickers()
	return nil
}
