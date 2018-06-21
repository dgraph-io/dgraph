/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package live

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgo/y"
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

	dc         *dgo.Dgraph
	alloc      *xidmap.XidMap
	ticker     *time.Ticker
	kv         *badger.DB
	requestsWg sync.WaitGroup
	// If we retry a request, we add one to retryRequestsWg.
	retryRequestsWg sync.WaitGroup

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

func handleError(err error) {
	errString := grpc.ErrorDesc(err)
	// Irrecoverable
	if strings.Contains(errString, "x509") || grpc.Code(err) == codes.Internal {
		x.Fatalf(errString)
	} else if errString != y.ErrAborted.Error() && errString != y.ErrConflict.Error() {
		x.Printf("Error while mutating %v\n", errString)
	}
}

func (l *loader) infinitelyRetry(req api.Mutation) {
	defer l.retryRequestsWg.Done()
	for i := time.Millisecond; ; i *= 2 {
		txn := l.dc.NewTxn()
		req.CommitNow = true
		_, err := txn.Mutate(l.opts.Ctx, &req)
		if err == nil {
			atomic.AddUint64(&l.rdfs, uint64(len(req.Set)))
			atomic.AddUint64(&l.txns, 1)
			return
		}
		handleError(err)
		atomic.AddUint64(&l.aborts, 1)
		if i >= 10*time.Second {
			i = 10 * time.Second
		}
		time.Sleep(i)
	}
}

func (l *loader) request(req api.Mutation) {
	txn := l.dc.NewTxn()
	req.CommitNow = true
	_, err := txn.Mutate(l.opts.Ctx, &req)

	if err == nil {
		atomic.AddUint64(&l.rdfs, uint64(len(req.Set)))
		atomic.AddUint64(&l.txns, 1)
		return
	}
	handleError(err)
	atomic.AddUint64(&l.aborts, 1)
	l.retryRequestsWg.Add(1)
	go l.infinitelyRetry(req)
}

// makeRequests can receive requests from batchNquads or directly from BatchSetWithMark.
// It doesn't need to batch the requests anymore. Batching is already done for it by the
// caller functions.
func (l *loader) makeRequests() {
	defer l.requestsWg.Done()
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
