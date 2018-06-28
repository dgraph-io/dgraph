/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package edgraph

import (
	"bytes"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"golang.org/x/net/context"
	"golang.org/x/net/trace"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgo/y"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/rdf"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
)

type ServerState struct {
	FinishCh   chan struct{} // channel to wait for all pending reqs to finish.
	ShutdownCh chan struct{} // channel to signal shutdown.

	Pstore   *badger.ManagedDB
	WALstore *badger.DB

	vlogTicker          *time.Ticker // runs every 1m, check size of vlog and run GC conditionally.
	mandatoryVlogTicker *time.Ticker // runs every 10m, we always run vlog GC.

	mu     sync.Mutex
	needTs []chan uint64
	notify chan struct{}
}

// TODO(tzdybal) - remove global
var State ServerState

func InitServerState() {
	Config.validate()

	State.FinishCh = make(chan struct{})
	State.ShutdownCh = make(chan struct{})
	State.notify = make(chan struct{}, 1)

	State.initStorage()

	go State.fillTimestampRequests()
}

func (s *ServerState) runVlogGC(store *badger.ManagedDB) {
	// Get initial size on start.
	_, lastVlogSize := store.Size()
	const GB = int64(1 << 30)

	runGC := func() {
		var err error
		for err == nil {
			// If a GC is successful, immediately run it again.
			err = store.RunValueLogGC(0.7)
		}
		_, lastVlogSize = store.Size()
	}

	for {
		select {
		case <-s.vlogTicker.C:
			_, currentVlogSize := store.Size()
			if currentVlogSize < lastVlogSize+GB {
				continue
			}
			runGC()
		case <-s.mandatoryVlogTicker.C:
			runGC()
		}
	}
}

func (s *ServerState) initStorage() {
	// Write Ahead Log directory
	x.Checkf(os.MkdirAll(Config.WALDir, 0700), "Error while creating WAL dir.")
	kvOpt := badger.DefaultOptions
	kvOpt.SyncWrites = true
	kvOpt.Dir = Config.WALDir
	kvOpt.ValueDir = Config.WALDir
	kvOpt.TableLoadingMode = options.MemoryMap

	var err error
	s.WALstore, err = badger.Open(kvOpt)
	x.Checkf(err, "Error while creating badger KV WAL store")

	// Postings directory
	// All the writes to posting store should be synchronous. We use batched writers
	// for posting lists, so the cost of sync writes is amortized.
	x.Check(os.MkdirAll(Config.PostingDir, 0700))
	x.Printf("Setting Badger option: %s", Config.BadgerOptions)
	var opt badger.Options
	switch Config.BadgerOptions {
	case "default":
		opt = badger.DefaultOptions
	case "lsmonly":
		opt = badger.LSMOnlyOptions
	default:
		x.Fatalf("Invalid Badger options")
	}
	opt.SyncWrites = true
	opt.Dir = Config.PostingDir
	opt.ValueDir = Config.PostingDir
	opt.NumVersionsToKeep = math.MaxInt32

	x.Printf("Setting Badger table load option: %s", Config.BadgerTables)
	switch Config.BadgerTables {
	case "mmap":
		opt.TableLoadingMode = options.MemoryMap
	case "ram":
		opt.TableLoadingMode = options.LoadToRAM
	case "disk":
		opt.TableLoadingMode = options.FileIO
	default:
		x.Fatalf("Invalid Badger Tables options")
	}

	x.Printf("Setting Badger value log load option: %s", Config.BadgerVlog)
	switch Config.BadgerVlog {
	case "mmap":
		opt.ValueLogLoadingMode = options.MemoryMap
	case "disk":
		opt.ValueLogLoadingMode = options.FileIO
	default:
		x.Fatalf("Invalid Badger Value log options")
	}

	x.Printf("Opening postings Badger DB with options: %+v\n", opt)
	s.Pstore, err = badger.OpenManaged(opt)
	x.Checkf(err, "Error while creating badger KV posting store")
	s.vlogTicker = time.NewTicker(1 * time.Minute)
	s.mandatoryVlogTicker = time.NewTicker(10 * time.Minute)
	go s.runVlogGC(s.Pstore)

	wrapper := &badger.ManagedDB{DB: s.WALstore}
	go s.runVlogGC(wrapper)
}

func (s *ServerState) Dispose() error {
	if err := s.Pstore.Close(); err != nil {
		return errors.Wrapf(err, "While closing postings store")
	}
	if err := s.WALstore.Close(); err != nil {
		return errors.Wrapf(err, "While closing WAL store")
	}
	s.vlogTicker.Stop()
	s.mandatoryVlogTicker.Stop()
	return nil
}

// Server implements protos.DgraphServer
type Server struct{}

// TODO(pawan) - Remove this logic from client after client doesn't have to fetch ts
// for Commit API.
func (s *ServerState) fillTimestampRequests() {
	var chs []chan uint64
	const (
		initDelay = 10 * time.Millisecond
		maxDelay  = 10 * time.Second
	)
	delay := initDelay
	for range s.notify {
	RETRY:
		s.mu.Lock()
		chs = append(chs, s.needTs...)
		s.needTs = s.needTs[:0]
		s.mu.Unlock()

		if len(chs) == 0 {
			continue
		}
		num := &intern.Num{Val: uint64(len(chs))}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		ts, err := worker.Timestamps(ctx, num)
		cancel()
		if err != nil {
			log.Printf("Error while retrieving timestamps: %v. Will retry...\n", err)
			time.Sleep(delay)
			delay *= 2
			if delay > maxDelay {
				delay = maxDelay
			}
			goto RETRY
		}
		delay = initDelay
		x.AssertTrue(ts.EndId-ts.StartId+1 == uint64(len(chs)))
		for i, ch := range chs {
			ch <- ts.StartId + uint64(i)
		}
		chs = chs[:0]
	}
}

func (s *ServerState) getTimestamp() uint64 {
	ch := make(chan uint64)
	s.mu.Lock()
	s.needTs = append(s.needTs, ch)
	s.mu.Unlock()

	select {
	case s.notify <- struct{}{}:
	default:
	}
	return <-ch
}

func (s *Server) Alter(ctx context.Context, op *api.Operation) (*api.Payload, error) {
	if op.Schema == "" && op.DropAttr == "" && !op.DropAll {
		// Must have at least one field set. This helps users if they attempt
		// to set a field but use the wrong name (could be decoded from JSON).
		return nil, x.Errorf("Operation must have at least one field set")
	}

	empty := &api.Payload{}
	if err := x.HealthCheck(); err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Request rejected %v", err)
		}
		return empty, err
	}

	if !isMutationAllowed(ctx) {
		return nil, x.Errorf("No mutations allowed.")
	}

	// StartTs is not needed if the predicate to be dropped lies on this server but is required
	// if it lies on some other machine. Let's get it for safety.
	m := &intern.Mutations{StartTs: State.getTimestamp()}
	if op.DropAll {
		m.DropAll = true
		_, err := query.ApplyMutations(ctx, m)
		return empty, err
	}
	if len(op.DropAttr) > 0 {
		nq := &api.NQuad{
			Subject:     x.Star,
			Predicate:   op.DropAttr,
			ObjectValue: &api.Value{&api.Value_StrVal{x.Star}},
		}
		wnq := &gql.NQuad{nq}
		edge, err := wnq.ToDeletePredEdge()
		if err != nil {
			return empty, err
		}
		edges := []*intern.DirectedEdge{edge}
		m.Edges = edges
		_, err = query.ApplyMutations(ctx, m)
		return empty, err
	}
	updates, err := schema.Parse(op.Schema)
	if err != nil {
		return empty, err
	}
	x.Printf("Got schema: %+v\n", updates)
	// TODO: Maybe add some checks about the schema.
	m.Schema = updates
	_, err = query.ApplyMutations(ctx, m)
	return empty, err
}

func (s *Server) Mutate(ctx context.Context, mu *api.Mutation) (resp *api.Assigned, err error) {
	resp = &api.Assigned{}
	if err := x.HealthCheck(); err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Request rejected %v", err)
		}
		return resp, err
	}

	if !isMutationAllowed(ctx) {
		return nil, x.Errorf("No mutations allowed.")
	}
	if mu.StartTs == 0 {
		mu.StartTs = State.getTimestamp()
	}
	emptyMutation :=
		len(mu.GetSetJson()) == 0 && len(mu.GetDeleteJson()) == 0 &&
			len(mu.Set) == 0 && len(mu.Del) == 0 &&
			len(mu.SetNquads) == 0 && len(mu.DelNquads) == 0
	if emptyMutation {
		return resp, fmt.Errorf("empty mutation")
	}
	if rand.Float64() < worker.Config.Tracing {
		var tr trace.Trace
		tr, ctx = x.NewTrace("Server.Mutate", ctx)
		defer tr.Finish()
	}

	var l query.Latency
	l.Start = time.Now()
	gmu, err := parseMutationObject(mu)
	if err != nil {
		return resp, err
	}
	parseEnd := time.Now()
	l.Parsing = parseEnd.Sub(l.Start)
	defer func() {
		l.Processing = time.Since(parseEnd)
		resp.Latency = &api.Latency{
			ParsingNs:    uint64(l.Parsing.Nanoseconds()),
			ProcessingNs: uint64(l.Processing.Nanoseconds()),
		}
	}()

	newUids, err := query.AssignUids(ctx, gmu.Set)
	if err != nil {
		return resp, err
	}
	resp.Uids = query.ConvertUidsToHex(query.StripBlankNode(newUids))
	edges, err := query.ToInternal(gmu, newUids)
	if err != nil {
		return resp, err
	}

	m := &intern.Mutations{
		Edges:   edges,
		StartTs: mu.StartTs,
	}
	resp.Context, err = query.ApplyMutations(ctx, m)
	if !mu.CommitNow {
		if err == y.ErrConflict {
			err = status.Errorf(codes.FailedPrecondition, err.Error())
		}
		return resp, err
	}

	// The following logic is for committing immediately.
	if err != nil {
		// ApplyMutations failed. We now want to abort the transaction,
		// ignoring any error that might occur during the abort (the user would
		// care more about the previous error).
		ctxn := resp.Context
		ctxn.Aborted = true
		_, _ = worker.CommitOverNetwork(ctx, ctxn)

		if err == y.ErrConflict {
			// We have already aborted the transaction, so the error message should reflect that.
			return resp, y.ErrAborted
		}
		return resp, err
	}
	tr, ok := trace.FromContext(ctx)
	if ok {
		tr.LazyPrintf("Prewrites err: %v. Attempting to commit/abort immediately.", err)
	}
	ctxn := resp.Context
	// zero would assign the CommitTs
	cts, err := worker.CommitOverNetwork(ctx, ctxn)
	if ok {
		tr.LazyPrintf("Status of commit at ts: %d: %v", ctxn.StartTs, err)
	}
	if err != nil {
		if err == y.ErrAborted {
			err = status.Errorf(codes.Aborted, err.Error())
			resp.Context.Aborted = true
		}
		return resp, err
	}
	// CommitNow was true, no need to send keys.
	resp.Context.Keys = resp.Context.Keys[:0]
	resp.Context.CommitTs = cts
	return resp, nil
}

// This method is used to execute the query and return the response to the
// client as a protocol buffer message.
func (s *Server) Query(ctx context.Context, req *api.Request) (resp *api.Response, err error) {
	if err := x.HealthCheck(); err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Request rejected %v", err)
		}
		return resp, err
	}

	x.PendingQueries.Add(1)
	x.NumQueries.Add(1)
	defer x.PendingQueries.Add(-1)
	if ctx.Err() != nil {
		return resp, ctx.Err()
	}

	if rand.Float64() < worker.Config.Tracing {
		var tr trace.Trace
		tr, ctx = x.NewTrace("GrpcQuery", ctx)
		defer tr.Finish()
	}

	resp = new(api.Response)
	if len(req.Query) == 0 {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Empty query")
		}
		return resp, fmt.Errorf("empty query")
	}

	if Config.DebugMode {
		x.Printf("Received query: %+v\n", req.Query)
	}
	var l query.Latency
	l.Start = time.Now()
	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("Query received: %v, variables: %v", req.Query, req.Vars)
	}

	parsedReq, err := gql.Parse(gql.Request{
		Str:       req.Query,
		Variables: req.Vars,
	})
	if err != nil {
		return resp, err
	}

	if req.StartTs == 0 {
		req.StartTs = State.getTimestamp()
	}
	resp.Txn = &api.TxnContext{
		StartTs: req.StartTs,
	}

	var queryRequest = query.QueryRequest{
		Latency:  &l,
		GqlQuery: &parsedReq,
		ReadTs:   req.StartTs,
		LinRead:  req.LinRead,
	}

	// Core processing happens here.
	var er query.ExecuteResult
	if er, err = queryRequest.Process(ctx); err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Error while processing query: %+v", err)
		}
		return resp, x.Wrap(err)
	}
	resp.Schema = er.SchemaNode

	json, err := query.ToJson(&l, er.Subgraphs)
	if err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Error while converting to protocol buffer: %+v", err)
		}
		return resp, err
	}
	resp.Json = json

	gl := &api.Latency{
		ParsingNs:    uint64(l.Parsing.Nanoseconds()),
		ProcessingNs: uint64(l.Processing.Nanoseconds()),
		EncodingNs:   uint64(l.Json.Nanoseconds()),
	}

	resp.Latency = gl
	resp.Txn.LinRead = queryRequest.LinRead
	return resp, err
}

func (s *Server) CommitOrAbort(ctx context.Context, tc *api.TxnContext) (*api.TxnContext,
	error) {
	if err := x.HealthCheck(); err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Request rejected %v", err)
		}
		return &api.TxnContext{}, err
	}

	tctx := &api.TxnContext{}

	if tc.StartTs == 0 {
		return &api.TxnContext{}, fmt.Errorf("StartTs cannot be zero while committing a transaction.")
	}
	commitTs, err := worker.CommitOverNetwork(ctx, tc)
	if err == y.ErrAborted {
		tctx.Aborted = true
		return tctx, status.Errorf(codes.Aborted, err.Error())
	}
	tctx.CommitTs = commitTs
	return tctx, err
}

func (s *Server) CheckVersion(ctx context.Context, c *api.Check) (v *api.Version, err error) {
	if err := x.HealthCheck(); err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("request rejected %v", err)
		}
		return v, err
	}

	v = new(api.Version)
	v.Tag = x.Version()
	return v, nil
}

//-------------------------------------------------------------------------------------------------
// HELPER FUNCTIONS
//-------------------------------------------------------------------------------------------------
func isMutationAllowed(ctx context.Context) bool {
	if !Config.Nomutations {
		return true
	}
	shareAllowed, ok := ctx.Value("_share_").(bool)
	if !ok || !shareAllowed {
		return false
	}
	return true
}

func parseNQuads(b []byte) ([]*api.NQuad, error) {
	var nqs []*api.NQuad
	for _, line := range bytes.Split(b, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		nq, err := rdf.Parse(string(line))
		if err == rdf.ErrEmpty {
			continue
		}
		if err != nil {
			return nil, err
		}
		nqs = append(nqs, &nq)
	}
	return nqs, nil
}

func parseMutationObject(mu *api.Mutation) (*gql.Mutation, error) {
	res := &gql.Mutation{}
	if len(mu.SetJson) > 0 {
		nqs, err := nquadsFromJson(mu.SetJson, set)
		if err != nil {
			return nil, err
		}
		res.Set = append(res.Set, nqs...)
	}
	if len(mu.DeleteJson) > 0 {
		nqs, err := nquadsFromJson(mu.DeleteJson, delete)
		if err != nil {
			return nil, err
		}
		res.Del = append(res.Del, nqs...)
	}
	if len(mu.SetNquads) > 0 {
		nqs, err := parseNQuads(mu.SetNquads)
		if err != nil {
			return nil, err
		}
		res.Set = append(res.Set, nqs...)
	}
	if len(mu.DelNquads) > 0 {
		nqs, err := parseNQuads(mu.DelNquads)
		if err != nil {
			return nil, err
		}
		res.Del = append(res.Del, nqs...)
	}

	// We check that the facet value is in the right format based on the facet type.
	for _, m := range mu.Set {
		for _, f := range m.Facets {
			if err := facets.TryValFor(f); err != nil {
				return nil, err
			}
		}
	}

	res.Set = append(res.Set, mu.Set...)
	res.Del = append(res.Del, mu.Del...)

	return res, validWildcards(res.Set, res.Del)
}

func validWildcards(set, del []*api.NQuad) error {
	for _, nq := range set {
		var ostar bool
		if o, ok := nq.ObjectValue.GetVal().(*api.Value_DefaultVal); ok {
			ostar = o.DefaultVal == x.Star
		}
		if nq.Subject == x.Star || nq.Predicate == x.Star || ostar {
			return x.Errorf("Cannot use star in set n-quad: %+v", nq)
		}
	}
	for _, nq := range del {
		var ostar bool
		if o, ok := nq.ObjectValue.GetVal().(*api.Value_DefaultVal); ok {
			ostar = o.DefaultVal == x.Star
		}
		if nq.Subject == x.Star || (nq.Predicate == x.Star && !ostar) {
			return x.Errorf("Only valid wildcard delete patterns are 'S * *' and 'S P *': %v", nq)
		}
	}
	return nil
}
