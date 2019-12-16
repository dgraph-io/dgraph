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
	"github.com/dgraph-io/dgo/v2"
	"github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/dgraph/cmd/zero"
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

	conflicts map[uint64]bool
	uidsLock  sync.RWMutex

	reqNum   uint64
	reqs     chan api.Mutation
	zeroconn *grpc.ClientConn
	sch      *schema
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
		if !isRetry && opt.verbose {
			fmt.Printf("Transaction #%d aborted. Will retry in background.\n", reqNum)
		}
	case strings.Contains(s.Message(), "Server overloaded."):
		dur := time.Duration(1+rand.Intn(10)) * time.Minute
		fmt.Printf("Server is overloaded. Will retry after %s.\n", dur.Round(time.Minute))
		time.Sleep(dur)
	case err != zero.ErrConflict && err != dgo.ErrAborted:
		fmt.Printf("Error while mutating: %v s.Code %v\n", s.Message(), s.Code())
	}
}

func (l *loader) infinitelyRetry(req api.Mutation, reqNum uint64) {
	defer l.retryRequestsWg.Done()
	defer l.deregister(&req)
	nretries := 1
	for i := time.Millisecond; ; i *= 2 {
		txn := l.dc.NewTxn()
		req.CommitNow = true
		_, err := txn.Mutate(l.opts.Ctx, &req)
		if err == nil {
			if opt.verbose {
				fmt.Printf("Transaction #%d succeeded after %s.\n",
					reqNum, english.Plural(nretries, "retry", "retries"))
			}
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
		l.deregister(&req)
		return
	}
	handleError(err, reqNum, false)
	atomic.AddUint64(&l.aborts, 1)
	l.retryRequestsWg.Add(1)
	go l.infinitelyRetry(req, reqNum)
}

func typeValFrom(val *api.Value) (types.Val, error) {
	var p types.Val
	switch val.Val.(type) {
	case *api.Value_BytesVal:
		p = types.Val{Tid: types.BinaryID, Value: val.GetBytesVal()}
	case *api.Value_IntVal:
		p = types.Val{Tid: types.IntID, Value: val.GetIntVal()}
	case *api.Value_StrVal:
		p = types.Val{Tid: types.StringID, Value: val.GetStrVal()}
	case *api.Value_BoolVal:
		p = types.Val{Tid: types.BoolID, Value: val.GetBoolVal()}
	case *api.Value_DoubleVal:
		p = types.Val{Tid: types.FloatID, Value: val.GetDoubleVal()}
	case *api.Value_GeoVal:
		p = types.Val{Tid: types.GeoID, Value: val.GetGeoVal()}
	case *api.Value_DatetimeVal:
		p = types.Val{Tid: types.DateTimeID, Value: val.GetDatetimeVal()}
	case *api.Value_PasswordVal:
		p = types.Val{Tid: types.PasswordID, Value: val.GetPasswordVal()}
	case *api.Value_DefaultVal:
		p = types.Val{Tid: types.DefaultID, Value: val.GetDefaultVal()}
	default:
		p = types.Val{Tid: types.StringID, Value: ""}
	}

	if p.Tid == types.GeoID || p.Tid == types.DateTimeID {
		p.Value = p.Value.([]byte)
		return p, nil
	}

	p1 := types.ValueForType(types.BinaryID)
	if err := types.Marshal(p, &p1); err != nil {
		return p1, err
	}

	p1.Value = p1.Value.([]byte)
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
	val, err := typeValFrom(nq.ObjectValue)
	if err == nil {
		p.Value = val.Value.([]byte)
		p.ValueType = val.Tid.Enum()
		return p, nil
	}
	return p, err
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

func (l *loader) getConflictKeys(nq *api.NQuad) []uint64 {
	sid, _ := strconv.ParseUint(nq.Subject, 0, 64)

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

	keys := make([]uint64, 0, 1)

	pred, ok := l.sch.preds[nq.Predicate]
	if !ok {
		return keys
	}

	if pred.List {
		key := fingerprintEdge(de, pred)
		keys = append(keys, farm.Fingerprint64(x.DataKey(nq.Predicate, sid))^key)
	} else {
		keys = append(keys, farm.Fingerprint64(x.DataKey(nq.Predicate, sid)))
	}

	if pred.Reverse {
		oi, _ := strconv.ParseUint(nq.ObjectId, 0, 64)
		keys = append(keys, farm.Fingerprint64(x.DataKey(nq.Predicate, oi)))
	}

	if nq.ObjectValue == nil || !(pred.Count || pred.Index) {
		return keys
	}

	for _, tokerName := range pred.Tokenizer {
		toker, ok := tok.GetTokenizer(tokerName)
		if !ok {
			fmt.Printf("unknown tokenizer %q", tokerName)
		}

		storageVal := types.Val{
			Tid:   types.TypeID(de.GetValueType()),
			Value: de.GetValue(),
		}

		schemaVal, err := types.Convert(storageVal, types.TypeID(pred.ValueType))
		x.Check(err)
		toks, err := tok.BuildTokens(schemaVal.Value, tok.GetLangTokenizer(toker, nq.Lang))
		x.Check(err)

		for _, t := range toks {
			keys = append(keys, farm.Fingerprint64(x.IndexKey(nq.Predicate, t))^sid)
		}

	}

	return keys
}

func (l *loader) getConflicts(req *api.Mutation) []uint64 {
	keys := make([]uint64, 0, len(req.Set))
	for _, nq := range req.Set {
		keys = append(keys, l.getConflictKeys(nq)...)
	}
	return keys
}

func (l *loader) addConflictKeys(req *api.Mutation) bool {
	keys := l.getConflicts(req)

	l.uidsLock.Lock()
	defer l.uidsLock.Unlock()

	for _, key := range keys {
		if t, ok := l.conflicts[key]; ok && t {
			return false
		}
	}

	for _, key := range keys {
		l.conflicts[key] = true
	}

	return true
}

func (l *loader) deregister(req *api.Mutation) {
	keys := l.getConflicts(req)

	l.uidsLock.Lock()
	defer l.uidsLock.Unlock()

	for _, i := range keys {
		delete(l.conflicts, i)
	}
}

// makeRequests can receive requests from batchNquads or directly from BatchSetWithMark.
// It doesn't need to batch the requests anymore. Batching is already done for it by the
// caller functions.
func (l *loader) makeRequests() {
	defer l.requestsWg.Done()

	buffer := make([]api.Mutation, 0, l.opts.bufferSize)

	drain := func(min int) {
		for len(buffer) > min {
			i := 0
			for _, mu := range buffer {
				if l.addConflictKeys(&mu) {
					reqNum := atomic.AddUint64(&l.reqNum, 1)
					l.request(mu, reqNum)
					continue
				}
				buffer[i] = mu
				i++
			}
			buffer = buffer[:i]
		}

	}

	for req := range l.reqs {
		if l.addConflictKeys(&req) {
			reqNum := atomic.AddUint64(&l.reqNum, 1)
			l.request(req, reqNum)
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
