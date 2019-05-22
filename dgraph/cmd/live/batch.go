/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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

package live

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgo/y"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/dgraph/xidmap"
	"github.com/dustin/go-humanize/english"
)

var (
	ErrMaxTries = errors.New("Max retries exceeded for request while doing batch mutations")
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

// loader is the data structure held by the user program for all interactions with the Dgraph
// server.  After making grpc connection a new Dgraph is created by function NewDgraphClient.
type loader struct {
	opts batchMutationOptions

	dc         *dgo.Dgraph
	alloc      *xidmap.XidMap
	ticker     *time.Ticker
	db         *badger.DB
	requestsWg sync.WaitGroup
	// If we retry a request, we add one to retryRequestsWg.
	retryRequestsWg sync.WaitGroup

	// Miscellaneous information to print counters.
	// Num of N-Quads sent
	nquads uint64
	// Num of txns sent
	txns uint64
	// Num of aborts
	aborts uint64
	// To get time elapsed
	start time.Time

	reqNum   uint64
	reqs     chan api.Mutation
	zeroconn *grpc.ClientConn
}

type uidProvider struct {
	zero pb.ZeroClient
	ctx  context.Context
}

func (p *uidProvider) ReserveUidRange() (uint64, uint64, error) {
	factor := time.Second
	for {
		assignedIds, err := p.zero.AssignUids(context.Background(), &pb.Num{Val: 1000})
		if err == nil {
			return assignedIds.StartId, assignedIds.EndId, nil
		}
		fmt.Printf("Error while getting lease %v\n", err)
		select {
		case <-time.After(factor):
		case <-p.ctx.Done():
			return 0, 0, p.ctx.Err()
		}
		if factor < 256*time.Second {
			factor *= 2
		}
	}
}

// Counter keeps a track of various parameters about a batch mutation. Running totals are printed
// if BatchMutationOptions PrintCounters is set to true.
type Counter struct {
	// Number of N-Quads processed by server.
	Nquads uint64
	// Number of mutations processed by the server.
	TxnsDone uint64
	// Number of Aborts
	Aborts uint64
	// Time elapsed since the batch started.
	Elapsed time.Duration
}

// handleError inspects errors and terminates if the errors are non-recoverable.
// A gRPC code is Internal if there is an unforeseen issue that needs attention.
// A gRPC code is Unavailable when we can't possibly reach the remote server, most likely the
// server expects TLS and our certificate does not match or the host name is not verified. When
// the node certificate is created the name much match the request host name. e.g., localhost not
// 127.0.0.1.
func handleError(err error, reqNum uint64, isRetry bool) {
	s := status.Convert(err)
	switch {
	case s.Code() == codes.Internal, s.Code() == codes.Unavailable:
		x.Fatalf(s.Message())
	case strings.Contains(s.Message(), "x509"):
		x.Fatalf(s.Message())
	case s.Code() == codes.Aborted:
		if !isRetry {
			fmt.Printf("Transaction #%d aborted. Will retry in background.\n", reqNum)
		}
	case strings.Contains(s.Message(), "Server overloaded."):
		dur := time.Duration(1+rand.Intn(10)) * time.Minute
		fmt.Printf("Server is overloaded. Will retry after %s.\n", dur.Round(time.Minute))
		time.Sleep(dur)
	case err != y.ErrConflict:
		fmt.Printf("Error while mutating: %v\n", s.Message())
	}
}

func (l *loader) infinitelyRetry(req api.Mutation, reqNum uint64) {
	defer l.retryRequestsWg.Done()
	nretries := 1
	for i := time.Millisecond; ; i *= 2 {
		txn := l.dc.NewTxn()
		req.CommitNow = true
		_, err := txn.Mutate(l.opts.Ctx, &req)
		if err == nil {
			fmt.Printf("Transaction #%d succeeded after %s.\n",
				reqNum, english.Plural(nretries, "retry", "retries"))
			atomic.AddUint64(&l.nquads, uint64(len(req.Set)))
			atomic.AddUint64(&l.txns, 1)
			return
		}
		nretries++
		handleError(err, reqNum, true)
		atomic.AddUint64(&l.aborts, 1)
		if i >= 10*time.Second {
			i = 10 * time.Second
		}
		time.Sleep(i)
	}
}

func (l *loader) request(req api.Mutation, reqNum uint64) {
	txn := l.dc.NewTxn()
	req.CommitNow = true
	_, err := txn.Mutate(l.opts.Ctx, &req)

	if err == nil {
		atomic.AddUint64(&l.nquads, uint64(len(req.Set)))
		atomic.AddUint64(&l.txns, 1)
		return
	}
	handleError(err, reqNum, false)
	atomic.AddUint64(&l.aborts, 1)
	l.retryRequestsWg.Add(1)
	go l.infinitelyRetry(req, reqNum)
}

// makeRequests can receive requests from batchNquads or directly from BatchSetWithMark.
// It doesn't need to batch the requests anymore. Batching is already done for it by the
// caller functions.
func (l *loader) makeRequests() {
	defer l.requestsWg.Done()
	for req := range l.reqs {
		reqNum := atomic.AddUint64(&l.reqNum, 1)
		l.request(req, reqNum)
	}
}

func (l *loader) printCounters() {
	period := 5 * time.Second
	l.ticker = time.NewTicker(period)
	start := time.Now()

	var last Counter
	for range l.ticker.C {
		counter := l.Counter()
		rate := float64(counter.Nquads-last.Nquads) / period.Seconds()
		elapsed := time.Since(start).Round(time.Second)
		timestamp := time.Now().Format("15:04:05Z0700")
		fmt.Printf("[%s] Elapsed: %s Txns: %d N-Quads: %d N-Quads/s [last 5s]: %5.0f Aborts: %d\n",
			timestamp, x.FixedDuration(elapsed), counter.TxnsDone, counter.Nquads, rate, counter.Aborts)
		last = counter
	}
}

// Counter returns the current state of the BatchMutation.
func (l *loader) Counter() Counter {
	return Counter{
		Nquads:   atomic.LoadUint64(&l.nquads),
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
