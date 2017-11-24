/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package live

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/protos/api"
	"github.com/dgraph-io/dgraph/protos/intern"
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
	zero intern.ZeroClient
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
	wg     sync.WaitGroup

	// Miscellaneous information to print counters.
	// Num of RDF's sent
	rdfs uint64
	// Num of txns sent
	txns uint64
	// Num of aborts
	aborts uint64
	// To get time elapsel.
	start time.Time

	reqs     chan api.Mutation
	zeroconn *grpc.ClientConn
}

func (p *uidProvider) ReserveUidRange() (start, end uint64, err error) {
	factor := time.Second
	for {
		assignedIds, err := p.zero.AssignUids(context.Background(), &intern.Num{Val: 1000})
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
	// Time elapsed since the batch started.
	Elapsed time.Duration
}

func (l *loader) infinitelyRetry(req api.Mutation) {
	defer l.wg.Done()
	for {
		txn := l.dc.NewTxn()
		req.CommitNow = true
		_, err := txn.Mutate(l.opts.Ctx, &req)
		if err == nil {
			atomic.AddUint64(&l.txns, 1)
			return
		}
		atomic.AddUint64(&l.aborts, 1)
		time.Sleep(10 * time.Millisecond)
	}
}

func (l *loader) request(req api.Mutation) {
	txn := l.dc.NewTxn()
	req.CommitNow = true
	_, err := txn.Mutate(l.opts.Ctx, &req)

	if err == nil {
		atomic.AddUint64(&l.txns, 1)
		return
	}
	atomic.AddUint64(&l.aborts, 1)
	l.wg.Add(1)
	go l.infinitelyRetry(req)
}

// makeRequests can receive requests from batchNquads or directly from BatchSetWithMark.
// It doesn't need to batch the requests anymore. Batching is already done for it by the
// caller functions.
func (l *loader) makeRequests() {
	defer l.wg.Done()
	for req := range l.reqs {
		l.request(req)
	}
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
