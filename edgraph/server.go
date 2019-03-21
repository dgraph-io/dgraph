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

package edgraph

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgo/y"

	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/rdf"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"

	"github.com/golang/glog"
	otrace "go.opencensus.io/trace"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type ServerState struct {
	FinishCh   chan struct{} // channel to wait for all pending reqs to finish.
	ShutdownCh chan struct{} // channel to signal shutdown.

	Pstore   *badger.DB
	WALstore *badger.DB

	vlogTicker          *time.Ticker // runs every 1m, check size of vlog and run GC conditionally.
	mandatoryVlogTicker *time.Ticker // runs every 10m, we always run vlog GC.

	needTs chan tsReq
}

var State ServerState

func InitServerState() {
	Config.validate()

	State.FinishCh = make(chan struct{})
	State.ShutdownCh = make(chan struct{})
	State.needTs = make(chan tsReq, 100)

	State.initStorage()

	go State.fillTimestampRequests()
}

func (s *ServerState) runVlogGC(store *badger.DB) {
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

func setBadgerOptions(opt badger.Options, dir string) badger.Options {
	opt.SyncWrites = true
	opt.Truncate = true
	opt.Dir = dir
	opt.ValueDir = dir
	opt.Logger = &conn.ToGlog{}

	glog.Infof("Setting Badger table load option: %s", Config.BadgerTables)
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

	glog.Infof("Setting Badger value log load option: %s", Config.BadgerVlog)
	switch Config.BadgerVlog {
	case "mmap":
		opt.ValueLogLoadingMode = options.MemoryMap
	case "disk":
		opt.ValueLogLoadingMode = options.FileIO
	default:
		x.Fatalf("Invalid Badger Value log options")
	}
	return opt
}

func (s *ServerState) initStorage() {
	var err error
	{
		// Write Ahead Log directory
		x.Checkf(os.MkdirAll(Config.WALDir, 0700), "Error while creating WAL dir.")
		opt := badger.LSMOnlyOptions
		opt = setBadgerOptions(opt, Config.WALDir)
		opt.ValueLogMaxEntries = 10000 // Allow for easy space reclamation.

		// We should always force load LSM tables to memory, disregarding user settings, because
		// Raft.Advance hits the WAL many times. If the tables are not in memory, retrieval slows
		// down way too much, causing cluster membership issues. Because of prefix compression and
		// value separation provided by Badger, this is still better than using the memory based WAL
		// storage provided by the Raft library.
		opt.TableLoadingMode = options.LoadToRAM

		glog.Infof("Opening write-ahead log BadgerDB with options: %+v\n", opt)
		s.WALstore, err = badger.Open(opt)
		x.Checkf(err, "Error while creating badger KV WAL store")
	}
	{
		// Postings directory
		// All the writes to posting store should be synchronous. We use batched writers
		// for posting lists, so the cost of sync writes is amortized.
		x.Check(os.MkdirAll(Config.PostingDir, 0700))
		opt := badger.DefaultOptions
		opt.ValueThreshold = 1 << 10 // 1KB
		opt.NumVersionsToKeep = math.MaxInt32
		opt = setBadgerOptions(opt, Config.PostingDir)

		glog.Infof("Opening postings BadgerDB with options: %+v\n", opt)
		s.Pstore, err = badger.OpenManaged(opt)
		x.Checkf(err, "Error while creating badger KV posting store")
	}

	s.vlogTicker = time.NewTicker(1 * time.Minute)
	s.mandatoryVlogTicker = time.NewTicker(10 * time.Minute)
	go s.runVlogGC(s.Pstore)
	go s.runVlogGC(s.WALstore)
}

func (s *ServerState) Dispose() {
	if err := s.Pstore.Close(); err != nil {
		glog.Errorf("Error while closing postings store: %v", err)
	}
	if err := s.WALstore.Close(); err != nil {
		glog.Errorf("Error while closing WAL store: %v", err)
	}
	s.vlogTicker.Stop()
	s.mandatoryVlogTicker.Stop()
}

// Server implements protos.DgraphServer
type Server struct{}

func (s *ServerState) fillTimestampRequests() {
	const (
		initDelay = 10 * time.Millisecond
		maxDelay  = time.Second
	)

	var reqs []tsReq
	for {
		// Reset variables.
		reqs = reqs[:0]
		delay := initDelay

		req := <-s.needTs
	slurpLoop:
		for {
			reqs = append(reqs, req)
			select {
			case req = <-s.needTs:
			default:
				break slurpLoop
			}
		}

		// Generate the request.
		num := &pb.Num{}
		for _, r := range reqs {
			if r.readOnly {
				num.ReadOnly = true
			} else {
				num.Val++
			}
		}

		// Execute the request with infinite retries.
	retry:
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		ts, err := worker.Timestamps(ctx, num)
		cancel()
		if err != nil {
			glog.Warningf("Error while retrieving timestamps: %v with delay: %v."+
				" Will retry...\n", err, delay)
			time.Sleep(delay)
			delay *= 2
			if delay > maxDelay {
				delay = maxDelay
			}
			goto retry
		}
		var offset uint64
		for _, req := range reqs {
			if req.readOnly {
				req.ch <- ts.ReadOnly
			} else {
				req.ch <- ts.StartId + offset
				offset++
			}
		}
		x.AssertTrue(ts.StartId == 0 || ts.StartId+offset-1 == ts.EndId)
	}
}

type tsReq struct {
	readOnly bool
	// A one-shot chan which we can send a txn timestamp upon.
	ch chan uint64
}

func (s *ServerState) getTimestamp(readOnly bool) uint64 {
	tr := tsReq{readOnly: readOnly, ch: make(chan uint64)}
	s.needTs <- tr
	return <-tr.ch
}

func (s *Server) Alter(ctx context.Context, op *api.Operation) (*api.Payload, error) {
	ctx, span := otrace.StartSpan(ctx, "Server.Alter")
	defer span.End()
	span.Annotatef(nil, "Alter operation: %+v", op)

	// Always print out Alter operations because they are important and rare.
	glog.Infof("Received ALTER op: %+v", op)

	// The following code block checks if the operation should run or not.
	if op.Schema == "" && op.DropAttr == "" && !op.DropAll {
		// Must have at least one field set. This helps users if they attempt
		// to set a field but use the wrong name (could be decoded from JSON).
		return nil, x.Errorf("Operation must have at least one field set")
	}
	empty := &api.Payload{}
	if err := x.HealthCheck(); err != nil {
		return empty, err
	}
	if !isMutationAllowed(ctx) {
		return nil, x.Errorf("No mutations allowed by server.")
	}
	if err := isAlterAllowed(ctx); err != nil {
		glog.Warningf("Alter denied with error: %v\n", err)
		return nil, err
	}
	// All checks done.

	defer glog.Infof("ALTER op: %+v done", op)
	// StartTs is not needed if the predicate to be dropped lies on this server but is required
	// if it lies on some other machine. Let's get it for safety.
	m := &pb.Mutations{StartTs: State.getTimestamp(false)}
	if op.DropAll {
		m.DropAll = true
		_, err := query.ApplyMutations(ctx, m)
		return empty, err
	}
	if len(op.DropAttr) > 0 {
		nq := &api.NQuad{
			Subject:     x.Star,
			Predicate:   op.DropAttr,
			ObjectValue: &api.Value{Val: &api.Value_StrVal{StrVal: x.Star}},
		}
		wnq := &gql.NQuad{NQuad: nq}
		edge, err := wnq.ToDeletePredEdge()
		if err != nil {
			return empty, err
		}
		edges := []*pb.DirectedEdge{edge}
		m.Edges = edges
		_, err = query.ApplyMutations(ctx, m)
		return empty, err
	}
	updates, err := schema.Parse(op.Schema)
	if err != nil {
		return empty, err
	}
	glog.Infof("Got schema: %+v\n", updates)
	// TODO: Maybe add some checks about the schema.
	m.Schema = updates
	_, err = query.ApplyMutations(ctx, m)
	return empty, err
}

func annotateStartTs(span *otrace.Span, ts uint64) {
	span.Annotate([]otrace.Attribute{otrace.Int64Attribute("startTs", int64(ts))}, "")
}

func (s *Server) Mutate(ctx context.Context, mu *api.Mutation) (resp *api.Assigned, err error) {
	ctx, span := otrace.StartSpan(ctx, "Server.Mutate")
	defer span.End()

	resp = &api.Assigned{}
	if err := x.HealthCheck(); err != nil {
		return resp, err
	}

	if len(mu.SetJson) > 0 {
		span.Annotatef(nil, "Got JSON Mutation: %s", mu.SetJson)
	} else if len(mu.SetNquads) > 0 {
		span.Annotatef(nil, "Got NQuad Mutation: %s", mu.SetNquads)
	}

	if !isMutationAllowed(ctx) {
		return nil, x.Errorf("No mutations allowed.")
	}
	if mu.StartTs == 0 {
		mu.StartTs = State.getTimestamp(false)
	}
	annotateStartTs(span, mu.StartTs)
	emptyMutation :=
		len(mu.GetSetJson()) == 0 && len(mu.GetDeleteJson()) == 0 &&
			len(mu.Set) == 0 && len(mu.Del) == 0 &&
			len(mu.SetNquads) == 0 && len(mu.DelNquads) == 0
	if emptyMutation {
		return resp, fmt.Errorf("Empty mutation")
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

	m := &pb.Mutations{
		Edges:   edges,
		StartTs: mu.StartTs,
	}
	span.Annotatef(nil, "Applying mutations: %+v", m)
	resp.Context, err = query.ApplyMutations(ctx, m)
	span.Annotatef(nil, "Txn Context: %+v. Err=%v", resp.Context, err)
	if !mu.CommitNow {
		if err == y.ErrConflict {
			err = status.Error(codes.FailedPrecondition, err.Error())
		}
		return resp, err
	}

	// The following logic is for committing immediately.
	if err != nil {
		// ApplyMutations failed. We now want to abort the transaction,
		// ignoring any error that might occur during the abort (the user would
		// care more about the previous error).
		if resp.Context == nil {
			resp.Context = &api.TxnContext{StartTs: mu.StartTs}
		}
		resp.Context.Aborted = true
		_, _ = worker.CommitOverNetwork(ctx, resp.Context)

		if err == y.ErrConflict {
			// We have already aborted the transaction, so the error message should reflect that.
			return resp, y.ErrAborted
		}
		return resp, err
	}
	span.Annotatef(nil, "Prewrites err: %v. Attempting to commit/abort immediately.", err)
	ctxn := resp.Context
	// zero would assign the CommitTs
	cts, err := worker.CommitOverNetwork(ctx, ctxn)
	span.Annotatef(nil, "Status of commit at ts: %d: %v", ctxn.StartTs, err)
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
	if glog.V(3) {
		glog.Infof("Got a query: %+v", req)
	}
	ctx, span := otrace.StartSpan(ctx, "Server.Query")
	defer span.End()

	if err := x.HealthCheck(); err != nil {
		return resp, err
	}

	x.PendingQueries.Add(1)
	x.NumQueries.Add(1)
	defer x.PendingQueries.Add(-1)
	if ctx.Err() != nil {
		return resp, ctx.Err()
	}

	resp = new(api.Response)
	if len(req.Query) == 0 {
		span.Annotate(nil, "Empty query")
		return resp, fmt.Errorf("Empty query")
	}

	var l query.Latency
	l.Start = time.Now()
	span.Annotatef(nil, "Query received: %v", req)

	parsedReq, err := gql.Parse(gql.Request{
		Str:       req.Query,
		Variables: req.Vars,
	})
	if err != nil {
		return resp, err
	}

	// if err = validateQuery(parsedReq.Query); err != nil {
	// 	return resp, err
	// }

	var queryRequest = query.QueryRequest{
		Latency:  &l,
		GqlQuery: &parsedReq,
	}
	// Here we try our best effort to not contact Zero for a timestamp. If we succeed,
	// then we use the max known transaction ts value (from ProcessDelta) for a read-only query.
	// If we haven't processed any updates yet then fall back to getting TS from Zero.
	switch {
	case req.BestEffort:
		span.Annotate([]otrace.Attribute{otrace.BoolAttribute("be", true)}, "")
	case req.ReadOnly:
		span.Annotate([]otrace.Attribute{otrace.BoolAttribute("ro", true)}, "")
	default:
		span.Annotate([]otrace.Attribute{otrace.BoolAttribute("no", true)}, "")
	}
	if req.BestEffort {
		// Sanity: check that request is read-only too.
		if !req.ReadOnly {
			return resp, x.Errorf("A best effort query must be read-only.")
		}
		if req.StartTs == 0 {
			req.StartTs = posting.Oracle().MaxAssigned()
		}
		queryRequest.Cache = worker.NoTxnCache
	}
	if req.StartTs == 0 {
		req.StartTs = State.getTimestamp(req.ReadOnly)
	}
	queryRequest.ReadTs = req.StartTs
	resp.Txn = &api.TxnContext{StartTs: req.StartTs}
	annotateStartTs(span, req.StartTs)

	// Core processing happens here.
	var er query.ExecuteResult
	if er, err = queryRequest.Process(ctx); err != nil {
		return resp, x.Wrap(err)
	}
	resp.Schema = er.SchemaNode

	var js []byte
	if len(resp.Schema) > 0 {
		sort.Slice(resp.Schema, func(i, j int) bool {
			return resp.Schema[i].Predicate < resp.Schema[j].Predicate
		})
		js, err = json.Marshal(map[string]interface{}{"schema": resp.Schema})
	} else {
		js, err = query.ToJson(&l, er.Subgraphs)
	}
	if err != nil {
		return resp, err
	}
	resp.Json = js
	span.Annotatef(nil, "Response = %s", js)

	gl := &api.Latency{
		ParsingNs:    uint64(l.Parsing.Nanoseconds()),
		ProcessingNs: uint64(l.Processing.Nanoseconds()),
		EncodingNs:   uint64(l.Json.Nanoseconds()),
	}

	resp.Latency = gl
	return resp, err
}

func (s *Server) CommitOrAbort(ctx context.Context, tc *api.TxnContext) (*api.TxnContext, error) {
	ctx, span := otrace.StartSpan(ctx, "Server.CommitOrAbort")
	defer span.End()

	if err := x.HealthCheck(); err != nil {
		return &api.TxnContext{}, err
	}

	tctx := &api.TxnContext{}
	if tc.StartTs == 0 {
		return &api.TxnContext{}, fmt.Errorf(
			"StartTs cannot be zero while committing a transaction")
	}
	annotateStartTs(span, tc.StartTs)

	span.Annotatef(nil, "Txn Context received: %+v", tc)
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
	if Config.MutationsMode != DisallowMutations {
		return true
	}
	shareAllowed, ok := ctx.Value("_share_").(bool)
	if !ok || !shareAllowed {
		return false
	}
	return true
}

var errNoAuth = x.Errorf("No Auth Token found. Token needed for Alter operations.")

func isAlterAllowed(ctx context.Context) error {
	p, ok := peer.FromContext(ctx)
	if ok {
		glog.Infof("Got Alter request from %q\n", p.Addr)
	}
	if len(Config.AuthToken) == 0 {
		return nil
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return errNoAuth
	}
	tokens := md.Get("auth-token")
	if len(tokens) == 0 {
		return errNoAuth
	}
	if tokens[0] != Config.AuthToken {
		return x.Errorf("Provided auth token [%s] does not match. Permission denied.", tokens[0])
	}
	return nil
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

// parseMutationObject tries to consolidate fields of the api.Mutation into the
// corresponding field of the returned gql.Mutation. For example, the 3 fields,
// api.Mutation#SetJson, api.Mutation#SetNquads and api.Mutation#Set are consolidated into the
// gql.Mutation.Set field. Similarly the 3 fields api.Mutation#DeleteJson, api.Mutation#DelNquads
// and api.Mutation#Del are merged into the gql.Mutation#Del field.
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

	// parse facets and convert to the binary format so that
	// a field of type datetime like "2017-01-01" can be correctly encoded in the
	// marshaled binary format as done in the time.Marshal method
	if err := validateAndConvertFacets(mu); err != nil {
		return nil, err
	}

	res.Set = append(res.Set, mu.Set...)
	res.Del = append(res.Del, mu.Del...)

	if err := validateNQuads(res.Set, res.Del); err != nil {
		return nil, err
	}
	return res, nil
}

func validateAndConvertFacets(mu *api.Mutation) error {
	for _, m := range mu.Set {
		encodedFacets := make([]*api.Facet, 0, len(m.Facets))
		for _, f := range m.Facets {
			// try to interpret the value as binary first
			if _, err := facets.ValFor(f); err == nil {
				encodedFacets = append(encodedFacets, f)
			} else {
				encodedFacet, err := facets.FacetFor(f.Key, string(f.Value))
				if err != nil {
					return err
				}
				encodedFacets = append(encodedFacets, encodedFacet)
			}
		}

		m.Facets = encodedFacets
	}
	return nil
}

func validateNQuads(set, del []*api.NQuad) error {
	for _, nq := range set {
		var ostar bool
		if o, ok := nq.ObjectValue.GetVal().(*api.Value_DefaultVal); ok {
			ostar = o.DefaultVal == x.Star
		}
		if nq.Subject == x.Star || nq.Predicate == x.Star || ostar {
			return x.Errorf("Cannot use star in set n-quad: %+v", nq)
		}
		if err := validateKeys(nq); err != nil {
			return x.Errorf("Key error: %s: %+v", err, nq)
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
		// NOTE: we dont validateKeys() with delete to let users fix existing mistakes
		// with bad predicate forms. ex: foo@bar ~something
	}
	return nil
}

func validateKey(key string) error {
	switch {
	case len(key) == 0:
		return x.Errorf("Has zero length")
	case strings.ContainsAny(key, "~@"):
		return x.Errorf("Has invalid characters")
	case strings.IndexFunc(key, unicode.IsSpace) != -1:
		return x.Errorf("Must not contain spaces")
	}
	return nil
}

// validateKeys checks predicate and facet keys in Nquad for syntax errors.
func validateKeys(nq *api.NQuad) error {
	if err := validateKey(nq.Predicate); err != nil {
		return x.Errorf("predicate %q %s", nq.Predicate, err)
	}
	for i := range nq.Facets {
		if nq.Facets[i] == nil {
			continue
		}
		if err := validateKey(nq.Facets[i].Key); err != nil {
			return x.Errorf("Facet %q, %s", nq.Facets[i].Key, err)
		}
	}
	return nil
}
