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
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/dgo/v200"
	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/dgraph/cmd/zero"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/dgraph/xidmap"
	"github.com/dgryski/go-farm"
	"github.com/dustin/go-humanize/english"
)

// batchMutationOptions sets the clients batch mode to Pending number of buffers each of Size.
// Running counters of number of rdfs processed, total time and mutations per second are printed
// if PrintCounters is set true.  See Counter.
type batchMutationOptions struct {
	Size          int
	Pending       int
	PrintCounters bool
	MaxRetries    uint32
	// BufferSize is the number of requests that a live loader thread can store at a time
	bufferSize int
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

	conflicts map[uint64]struct{}
	uidsLock  sync.RWMutex

	reqNum   uint64
	reqs     chan *request
	zeroconn *grpc.ClientConn
	schema   *schema

	upsertLock sync.RWMutex
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
func handleError(err error, isRetry bool) {
	s := status.Convert(err)
	switch {
	case s.Code() == codes.Internal, s.Code() == codes.Unavailable:
		// Let us not crash live loader due to this. Instead, we should infinitely retry to
		// reconnect and retry the request.
		dur := time.Duration(1+rand.Intn(60)) * time.Second
		fmt.Printf("Connection has been possibly interrupted. Got error: %v."+
			" Will retry after %s.\n", err, dur.Round(time.Second))
		time.Sleep(dur)
	case strings.Contains(s.Message(), "x509"):
		x.Fatalf(s.Message())
	case s.Code() == codes.Aborted:
		if !isRetry && opt.verbose {
			fmt.Printf("Transaction aborted. Will retry in background.\n")
		}
	case strings.Contains(s.Message(), "Server overloaded."):
		dur := time.Duration(1+rand.Intn(10)) * time.Minute
		fmt.Printf("Server is overloaded. Will retry after %s.\n", dur.Round(time.Minute))
		time.Sleep(dur)
	case err != zero.ErrConflict && err != dgo.ErrAborted:
		fmt.Printf("Error while mutating: %v s.Code %v\n", s.Message(), s.Code())
	}
}

func (l *loader) infinitelyRetry(req *request) {
	defer l.retryRequestsWg.Done()
	defer l.deregister(req)
	nretries := 1
	for i := time.Millisecond; ; i *= 2 {
		err := l.mutate(req)
		if err == nil {
			if opt.verbose {
				fmt.Printf("Transaction succeeded after %s.\n",
					english.Plural(nretries, "retry", "retries"))
			}
			atomic.AddUint64(&l.nquads, uint64(len(req.Set)))
			atomic.AddUint64(&l.txns, 1)
			return
		}
		nretries++
		handleError(err, true)
		atomic.AddUint64(&l.aborts, 1)
		if i >= 10*time.Second {
			i = 10 * time.Second
		}
		time.Sleep(i)
	}
}

func (l *loader) mutate(req *request) error {
	txn := l.dc.NewTxn()
	req.CommitNow = true
	request := &api.Request{
		CommitNow: true,
		Mutations: []*api.Mutation{req.Mutation},
	}
	_, err := txn.Do(l.opts.Ctx, request)
	return err
}

func (l *loader) request(req *request) {
	atomic.AddUint64(&l.reqNum, 1)
	err := l.mutate(req)
	if err == nil {
		atomic.AddUint64(&l.nquads, uint64(len(req.Set)))
		atomic.AddUint64(&l.txns, 1)
		l.deregister(req)
		return
	}
	handleError(err, false)
	atomic.AddUint64(&l.aborts, 1)
	l.retryRequestsWg.Add(1)
	go l.infinitelyRetry(req)
}

func getTypeVal(val *api.Value) (types.Val, error) {
	p := gql.TypeValFrom(val)
	//Convert value to bytes

	if p.Tid == types.GeoID || p.Tid == types.DateTimeID {
		// Already in bytes format
		p.Value = p.Value.([]byte)
		return p, nil
	}

	p1 := types.ValueForType(types.BinaryID)
	if err := types.Marshal(p, &p1); err != nil {
		return p1, err
	}

	p1.Value = p1.Value.([]byte)
	p1.Tid = p.Tid
	return p1, nil
}

func createUidEdge(nq *api.NQuad, sid, oid uint64) *pb.DirectedEdge {
	return &pb.DirectedEdge{
		Entity:    sid,
		Attr:      nq.Predicate,
		Label:     nq.Label,
		Lang:      nq.Lang,
		Facets:    nq.Facets,
		ValueId:   oid,
		ValueType: pb.Posting_UID,
	}
}

func createValueEdge(nq *api.NQuad, sid uint64) (*pb.DirectedEdge, error) {
	p := &pb.DirectedEdge{
		Entity: sid,
		Attr:   nq.Predicate,
		Label:  nq.Label,
		Lang:   nq.Lang,
		Facets: nq.Facets,
	}
	val, err := getTypeVal(nq.ObjectValue)
	if err != nil {
		return p, err
	}

	p.Value = val.Value.([]byte)
	p.ValueType = val.Tid.Enum()
	return p, nil
}

func fingerprintEdge(t *pb.DirectedEdge, pred *predicate) uint64 {
	var id uint64 = math.MaxUint64

	// Value with a lang type.
	if len(t.Lang) > 0 {
		id = farm.Fingerprint64([]byte(t.Lang))
	} else if pred.List {
		id = farm.Fingerprint64(t.Value)
	}
	return id
}

func (l *loader) conflictKeysForNQuad(nq *api.NQuad) ([]uint64, error) {
	pred, found := l.schema.preds[nq.Predicate]

	// We dont' need to generate conflict keys for predicate with noconflict directive.
	if found && pred.NoConflict || opt.ludicrousMode {
		return nil, nil
	}

	keys := make([]uint64, 0)

	// Calculates the conflict keys, inspired by the logic in
	// addMutationInteration in posting/list.go.
	sid, err := strconv.ParseUint(nq.Subject, 0, 64)
	if err != nil {
		return nil, err
	}

	var oid uint64
	var de *pb.DirectedEdge

	if nq.ObjectValue == nil {
		oid, _ = strconv.ParseUint(nq.ObjectId, 0, 64)
		de = createUidEdge(nq, sid, oid)
	} else {
		var err error
		de, err = createValueEdge(nq, sid)
		x.Check(err)
	}

	// If the predicate is not found in schema then we don't have to generate any more keys.
	if !found {
		return keys, nil
	}

	if pred.List {
		key := fingerprintEdge(de, pred)
		keys = append(keys, farm.Fingerprint64(x.DataKey(nq.Predicate, sid))^key)
	} else {
		keys = append(keys, farm.Fingerprint64(x.DataKey(nq.Predicate, sid)))
	}

	if pred.Reverse {
		oi, err := strconv.ParseUint(nq.ObjectId, 0, 64)
		if err != nil {
			return keys, err
		}
		keys = append(keys, farm.Fingerprint64(x.DataKey(nq.Predicate, oi)))
	}

	if nq.ObjectValue == nil || !(pred.Count || pred.Index) {
		return keys, nil
	}

	errs := make([]string, 0)
	for _, tokName := range pred.Tokenizer {
		token, ok := tok.GetTokenizer(tokName)
		if !ok {
			fmt.Printf("unknown tokenizer %q", tokName)
			continue
		}

		storageVal := types.Val{
			Tid:   types.TypeID(de.GetValueType()),
			Value: de.GetValue(),
		}

		schemaVal, err := types.Convert(storageVal, types.TypeID(pred.ValueType))
		if err != nil {
			errs = append(errs, err.Error())
		}
		toks, err := tok.BuildTokens(schemaVal.Value, tok.GetTokenizerForLang(token, nq.Lang))
		if err != nil {
			errs = append(errs, err.Error())
		}

		for _, t := range toks {
			keys = append(keys, farm.Fingerprint64(x.IndexKey(nq.Predicate, t))^sid)
		}

	}

	if len(errs) > 0 {
		return keys, fmt.Errorf(strings.Join(errs, "\n"))
	}
	return keys, nil
}

func (l *loader) conflictKeysForReq(req *request) []uint64 {
	// Live loader only needs to look at sets and not deletes
	keys := make([]uint64, 0, len(req.Set))
	for _, nq := range req.Set {
		conflicts, err := l.conflictKeysForNQuad(nq)
		if err != nil {
			fmt.Println(err)
			continue
		}
		keys = append(keys, conflicts...)
	}
	return keys
}

func (l *loader) addConflictKeys(req *request) bool {
	l.uidsLock.Lock()
	defer l.uidsLock.Unlock()

	for _, key := range req.conflicts {
		if _, ok := l.conflicts[key]; ok {
			return false
		}
	}

	for _, key := range req.conflicts {
		l.conflicts[key] = struct{}{}
	}

	return true
}

func (l *loader) deregister(req *request) {
	l.uidsLock.Lock()
	defer l.uidsLock.Unlock()

	for _, i := range req.conflicts {
		delete(l.conflicts, i)
	}
}

// makeRequests can receive requests from batchNquads or directly from BatchSetWithMark.
// It doesn't need to batch the requests anymore. Batching is already done for it by the
// caller functions.
func (l *loader) makeRequests() {
	defer l.requestsWg.Done()

	buffer := make([]*request, 0, l.opts.bufferSize)
	drain := func(maxSize int) {
		for len(buffer) > maxSize {
			i := 0
			for _, req := range buffer {
				// If there is no conflict in req, we will use it
				// and then it would shift all the other reqs in buffer
				if !l.addConflictKeys(req) {
					buffer[i] = req
					i++
					continue
				}
				// Req will no longer be part of a buffer
				l.request(req)
			}
			buffer = buffer[:i]
		}
	}

	for req := range l.reqs {
		req.conflicts = l.conflictKeysForReq(req)
		if l.addConflictKeys(req) {
			l.request(req)
		} else {
			buffer = append(buffer, req)
		}
		drain(l.opts.bufferSize - 1)
	}

	drain(0)
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
