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
	"encoding/json"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/options"
	"github.com/dgraph-io/dgo/v2"
	"github.com/dgraph-io/dgo/v2/protos/api"

	"github.com/dgraph-io/dgraph/chunker"
	"github.com/dgraph-io/dgraph/dgraph/cmd/zero"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	ostats "go.opencensus.io/stats"
	"go.opencensus.io/tag"
	"go.opencensus.io/trace"
	otrace "go.opencensus.io/trace"
)

const (
	methodMutate = "Server.Mutate"
	methodQuery  = "Server.Query"
	groupFile    = "group_id"
)

// ServerState holds the state of the Dgraph server.
type ServerState struct {
	FinishCh   chan struct{} // channel to wait for all pending reqs to finish.
	ShutdownCh chan struct{} // channel to signal shutdown.

	Pstore   *badger.DB
	WALstore *badger.DB

	vlogTicker          *time.Ticker // runs every 1m, check size of vlog and run GC conditionally.
	mandatoryVlogTicker *time.Ticker // runs every 10m, we always run vlog GC.

	needTs chan tsReq
}

const (
	// NeedAuthorize is used to indicate that the request needs to be authorized.
	NeedAuthorize = iota
	// NoAuthorize is used to indicate that authorization needs to be skipped.
	// Used when ACL needs to query information for performing the authorization check.
	NoAuthorize
)

// State is the instance of ServerState used by the current server.
var State ServerState

// InitServerState initializes this server's state.
func InitServerState() {
	Config.validate()

	State.FinishCh = make(chan struct{})
	State.ShutdownCh = make(chan struct{})
	State.needTs = make(chan tsReq, 100)

	State.initStorage()
	go State.fillTimestampRequests()

	contents, err := ioutil.ReadFile(filepath.Join(Config.PostingDir, groupFile))
	if err != nil {
		return
	}

	glog.Infof("Found group_id file inside posting directory %s. Will attempt to read.",
		Config.PostingDir)
	groupId, err := strconv.ParseUint(strings.TrimSpace(string(contents)), 0, 32)
	if err != nil {
		glog.Warningf("Could not read %s file inside posting directory %s.",
			groupFile, Config.PostingDir)
	}
	x.WorkerConfig.ProposedGroupId = uint32(groupId)
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

func setBadgerOptions(opt badger.Options) badger.Options {
	opt = opt.WithSyncWrites(false).WithTruncate(true).WithLogger(&x.ToGlog{})

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
		opt := badger.LSMOnlyOptions(Config.WALDir)
		opt = setBadgerOptions(opt)
		opt.ValueLogMaxEntries = 10000 // Allow for easy space reclamation.
		opt.MaxCacheSize = 10 << 20    // 10 mb of cache size for WAL.

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
		opt := badger.DefaultOptions(Config.PostingDir).WithValueThreshold(1 << 10 /* 1KB */).
			WithNumVersionsToKeep(math.MaxInt32).WithMaxCacheSize(1 << 30)
		opt = setBadgerOptions(opt)

		glog.Infof("Opening postings BadgerDB with options: %+v\n", opt)
		s.Pstore, err = badger.OpenManaged(opt)
		x.Checkf(err, "Error while creating badger KV posting store")
	}

	s.vlogTicker = time.NewTicker(1 * time.Minute)
	s.mandatoryVlogTicker = time.NewTicker(10 * time.Minute)
	go s.runVlogGC(s.Pstore)
	go s.runVlogGC(s.WALstore)
}

// Dispose stops and closes all the resources inside the server state.
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

// Alter handles requests to change the schema or remove parts or all of the data.
func (s *Server) Alter(ctx context.Context, op *api.Operation) (*api.Payload, error) {
	ctx, span := otrace.StartSpan(ctx, "Server.Alter")
	defer span.End()
	span.Annotatef(nil, "Alter operation: %+v", op)

	// Always print out Alter operations because they are important and rare.
	glog.Infof("Received ALTER op: %+v", op)

	// The following code block checks if the operation should run or not.
	if op.Schema == "" && op.DropAttr == "" && !op.DropAll && op.DropOp == api.Operation_NONE {
		// Must have at least one field set. This helps users if they attempt
		// to set a field but use the wrong name (could be decoded from JSON).
		return nil, errors.Errorf("Operation must have at least one field set")
	}
	empty := &api.Payload{}
	if err := x.HealthCheck(); err != nil {
		return empty, err
	}

	if isDropAll(op) && op.DropOp == api.Operation_DATA {
		return nil, errors.Errorf("Only one of DropAll and DropData can be true")
	}

	if !isMutationAllowed(ctx) {
		return nil, errors.Errorf("No mutations allowed by server.")
	}
	if err := isAlterAllowed(ctx); err != nil {
		glog.Warningf("Alter denied with error: %v\n", err)
		return nil, err
	}

	if err := authorizeAlter(ctx, op); err != nil {
		glog.Warningf("Alter denied with error: %v\n", err)
		return nil, err
	}

	defer glog.Infof("ALTER op: %+v done", op)

	// StartTs is not needed if the predicate to be dropped lies on this server but is required
	// if it lies on some other machine. Let's get it for safety.
	m := &pb.Mutations{StartTs: State.getTimestamp(false)}
	if isDropAll(op) {
		if len(op.DropValue) > 0 {
			return empty, errors.Errorf("If DropOp is set to ALL, DropValue must be empty")
		}

		m.DropOp = pb.Mutations_ALL
		_, err := query.ApplyMutations(ctx, m)

		// recreate the admin account after a drop all operation
		ResetAcl()
		return empty, err
	}

	if op.DropOp == api.Operation_DATA {
		if len(op.DropValue) > 0 {
			return empty, errors.Errorf("If DropOp is set to DATA, DropValue must be empty")
		}

		m.DropOp = pb.Mutations_DATA
		_, err := query.ApplyMutations(ctx, m)

		// recreate the admin account after a drop data operation
		ResetAcl()
		return empty, err
	}

	if len(op.DropAttr) > 0 || op.DropOp == api.Operation_ATTR {
		if op.DropOp == api.Operation_ATTR && len(op.DropValue) == 0 {
			return empty, errors.Errorf("If DropOp is set to ATTR, DropValue must not be empty")
		}

		var attr string
		if len(op.DropAttr) > 0 {
			attr = op.DropAttr
		} else {
			attr = op.DropValue
		}

		// Reserved predicates cannot be dropped.
		if x.IsReservedPredicate(attr) {
			err := errors.Errorf("predicate %s is reserved and is not allowed to be dropped",
				attr)
			return empty, err
		}

		nq := &api.NQuad{
			Subject:     x.Star,
			Predicate:   attr,
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

	if op.DropOp == api.Operation_TYPE {
		if len(op.DropValue) == 0 {
			return empty, errors.Errorf("If DropOp is set to TYPE, DropValue must not be empty")
		}

		m.DropOp = pb.Mutations_TYPE
		m.DropValue = op.DropValue
		_, err := query.ApplyMutations(ctx, m)
		return empty, err
	}

	result, err := schema.Parse(op.Schema)
	if err != nil {
		return empty, err
	}

	for _, update := range result.Preds {
		// Reserved predicates cannot be altered but let the update go through
		// if the update is equal to the existing one.
		if schema.IsReservedPredicateChanged(update.Predicate, update) {
			err := errors.Errorf("predicate %s is reserved and is not allowed to be modified",
				update.Predicate)
			return nil, err
		}

		if err := validatePredName(update.Predicate); err != nil {
			return nil, err
		}
	}

	glog.Infof("Got schema: %+v\n", result)
	// TODO: Maybe add some checks about the schema.
	m.Schema = result.Preds
	m.Types = result.Types
	_, err = query.ApplyMutations(ctx, m)
	return empty, err
}

func annotateStartTs(span *otrace.Span, ts uint64) {
	span.Annotate([]otrace.Attribute{otrace.Int64Attribute("startTs", int64(ts))}, "")
}

func (s *Server) doMutate(ctx context.Context, qc *queryContext, resp *api.Response) error {
	if len(qc.gmuList) == 0 {
		return nil
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	start := time.Now()
	defer func() {
		qc.latency.Processing += time.Since(start)
	}()

	if !isMutationAllowed(ctx) {
		return errors.Errorf("no mutations allowed")
	}

	// update mutations from the query results before assigning UIDs
	updateMutations(qc)

	newUids, err := query.AssignUids(ctx, qc.gmuList)
	if err != nil {
		return err
	}

	// resp.Uids contains a map of the node name to the uid.
	// 1. For a blank node, like _:foo, the key would be foo.
	// 2. For a uid variable that is part of an upsert query,
	//    like uid(foo), the key would be uid(foo).
	resp.Uids = query.UidsToHex(query.StripBlankNode(newUids))
	edges, err := query.ToDirectedEdges(qc.gmuList, newUids)
	if err != nil {
		return err
	}

	m := &pb.Mutations{Edges: edges, StartTs: qc.req.StartTs}
	qc.span.Annotatef(nil, "Applying mutations: %+v", m)
	resp.Txn, err = query.ApplyMutations(ctx, m)
	qc.span.Annotatef(nil, "Txn Context: %+v. Err=%v", resp.Txn, err)
	if !qc.req.CommitNow {
		if err == zero.ErrConflict {
			err = status.Error(codes.FailedPrecondition, err.Error())
		}

		return err
	}

	// The following logic is for committing immediately.
	if err != nil {
		// ApplyMutations failed. We now want to abort the transaction,
		// ignoring any error that might occur during the abort (the user would
		// care more about the previous error).
		if resp.Txn == nil {
			resp.Txn = &api.TxnContext{StartTs: qc.req.StartTs}
		}

		resp.Txn.Aborted = true
		_, _ = worker.CommitOverNetwork(ctx, resp.Txn)

		if err == zero.ErrConflict {
			// We have already aborted the transaction, so the error message should reflect that.
			return dgo.ErrAborted
		}

		return err
	}

	qc.span.Annotatef(nil, "Prewrites err: %v. Attempting to commit/abort immediately.", err)
	ctxn := resp.Txn
	// zero would assign the CommitTs
	cts, err := worker.CommitOverNetwork(ctx, ctxn)
	qc.span.Annotatef(nil, "Status of commit at ts: %d: %v", ctxn.StartTs, err)
	if err != nil {
		if err == dgo.ErrAborted {
			err = status.Errorf(codes.Aborted, err.Error())
			resp.Txn.Aborted = true
		}

		return err
	}

	// CommitNow was true, no need to send keys.
	resp.Txn.Keys = resp.Txn.Keys[:0]
	resp.Txn.CommitTs = cts

	return nil
}

// buildUpsertQuery modifies the query to evaluate the
// @if condition defined in Conditional Upsert.
func buildUpsertQuery(qc *queryContext) string {
	if len(qc.req.Query) == 0 || len(qc.gmuList) == 0 {
		return qc.req.Query
	}

	qc.condVars = make([]string, len(qc.req.Mutations))
	upsertQuery := strings.TrimSuffix(qc.req.Query, "}")
	for i, gmu := range qc.gmuList {
		isCondUpsert := strings.TrimSpace(gmu.Cond) != ""
		if isCondUpsert {
			qc.condVars[i] = "__dgraph__" + strconv.Itoa(i)
			qc.uidRes[qc.condVars[i]] = nil
			// @if in upsert is same as @filter in the query
			cond := strings.Replace(gmu.Cond, "@if", "@filter", 1)

			// Add dummy query to evaluate the @if directive, ok to use uid(0) because
			// dgraph doesn't check for existence of UIDs until we query for other predicates.
			// Here, we are only querying for uid predicate in the dummy query.
			//
			// For example if - mu.Query = {
			//      me(...) {...}
			//   }
			//
			// Then, upsertQuery = {
			//      me(...) {...}
			//      __dgraph_0__ as var(func: uid(0)) @filter(...)
			//   }
			//
			// The variable __dgraph_0__ will -
			//      * be empty if the condition is true
			//      * have 1 UID (the 0 UID) if the condition is false
			upsertQuery += qc.condVars[i] + ` as var(func: uid(0)) ` + cond + `
			 `
		}
	}
	upsertQuery += `}`

	return upsertQuery
}

// updateMutations updates the mutation and replaces uid(var) and val(var) with
// their values or a blank node, in case of an upsert.
// We use the values stored in qc.uidRes and qc.valRes to update the mutation.
func updateMutations(qc *queryContext) {
	for i, condVar := range qc.condVars {
		gmu := qc.gmuList[i]
		if len(condVar) != 0 {
			uids, ok := qc.uidRes[condVar]
			if !(ok && len(uids) == 1) {
				gmu.Set = nil
				gmu.Del = nil
				continue
			}
		}

		updateUIDInMutations(gmu, qc)
		updateValInMutations(gmu, qc)
	}
}

// findMutationVars finds all the variables used in mutation block and stores them
// qc.uidRes and qc.valRes so that we only look for these variables in query results.
func findMutationVars(qc *queryContext) []string {
	updateVars := func(s string) {
		if strings.HasPrefix(s, "uid(") {
			varName := s[4 : len(s)-1]
			qc.uidRes[varName] = nil
		} else if strings.HasPrefix(s, "val(") {
			varName := s[4 : len(s)-1]
			qc.valRes[varName] = nil
		}
	}

	for _, gmu := range qc.gmuList {
		for _, nq := range gmu.Set {
			updateVars(nq.Subject)
			updateVars(nq.ObjectId)
		}
		for _, nq := range gmu.Del {
			updateVars(nq.Subject)
			updateVars(nq.ObjectId)
		}
	}

	varsList := make([]string, 0, len(qc.uidRes)+len(qc.valRes))
	for v := range qc.uidRes {
		varsList = append(varsList, v)
	}
	for v := range qc.valRes {
		varsList = append(varsList, v)
	}

	return varsList
}

// updateValInNQuads picks the val() from object and replaces it with its value
// Assumption is that Subject can contain UID, whereas Object can contain Val
// If val(variable) exists in a query, but the values are not there for the variable,
// it will ignore the mutation silently.
func updateValInNQuads(nquads []*api.NQuad, qc *queryContext) []*api.NQuad {
	getNewVals := func(s string) (map[uint64]types.Val, bool) {
		if strings.HasPrefix(s, "val(") {
			varName := s[4 : len(s)-1]
			if v, ok := qc.valRes[varName]; ok && v != nil {
				return v, true
			}
			return nil, true
		}
		return nil, false
	}

	getValue := func(key uint64, uidToVal map[uint64]types.Val) (types.Val, bool) {
		val, ok := uidToVal[key]
		if ok {
			return val, true
		}

		// Check if the variable is aggregate variable
		// Only 0 key would exist for aggregate variable
		val, ok = uidToVal[0]
		return val, ok
	}

	newNQuads := nquads[:0]
	for _, nq := range nquads {
		// Check if the nquad contains a val() in Object or not.
		// If not then, keep the mutation and continue
		uidToVal, found := getNewVals(nq.ObjectId)
		if !found {
			newNQuads = append(newNQuads, nq)
			continue
		}

		// uid(u) <amount> val(amt)
		// For each NQuad, we need to convert the val(variable_name)
		// to *api.Value before applying the mutation. For that, first
		// we convert key to uint64 and get the UID to Value map from
		// the result of the query.
		if nq.Subject[0] == '_' {
			// UID is of format "_:uid(u)". Ignore silently
			continue
		}

		key, err := strconv.ParseUint(nq.Subject, 0, 64)
		if err != nil {
			// Key conversion failed, ignoring the nquad. Ideally,
			// it shouldn't happen as this is the result of a query.
			glog.Errorf("Conversion of subject %s failed. Error: %s",
				nq.Subject, err.Error())
			continue
		}

		// Get the value to the corresponding UID(key) from the query result
		nq.ObjectId = ""
		val, ok := getValue(key, uidToVal)
		if !ok {
			continue
		}

		// Convert the value from types.Val to *api.Value
		nq.ObjectValue, err = types.ObjectValue(val.Tid, val.Value)
		if err != nil {
			// Value conversion failed, ignoring the nquad. Ideally,
			// it shouldn't happen as this is the result of a query.
			glog.Errorf("Conversion of %s failed for %d subject. Error: %s",
				nq.ObjectId, key, err.Error())
			continue
		}

		newNQuads = append(newNQuads, nq)
	}
	return newNQuads
}

// updateValInMuations does following transformations:
// 0x123 <amount> val(v) -> 0x123 <amount> 13.0
func updateValInMutations(gmu *gql.Mutation, qc *queryContext) {
	gmu.Del = updateValInNQuads(gmu.Del, qc)
	gmu.Set = updateValInNQuads(gmu.Set, qc)
}

// updateUIDInMutations does following transformations:
//   * uid(v) -> 0x123     -- If v is defined in query block
//   * uid(v) -> _:uid(v)  -- Otherwise
func updateUIDInMutations(gmu *gql.Mutation, qc *queryContext) {
	// usedMutationVars keeps track of variables that are used in mutations.
	getNewVals := func(s string) []string {
		if strings.HasPrefix(s, "uid(") {
			varName := s[4 : len(s)-1]
			if uids, ok := qc.uidRes[varName]; ok && len(uids) != 0 {
				return uids
			}

			return []string{"_:" + s}
		}

		return []string{s}
	}

	getNewNQuad := func(nq *api.NQuad, s, o string) *api.NQuad {
		// The following copy is fine because we only modify Subject and ObjectId.
		// The pointer values are not modified across different copies of NQuad.
		n := *nq

		n.Subject = s
		n.ObjectId = o
		return &n
	}

	// Remove the mutations from gmu.Del when no UID was found.
	gmuDel := make([]*api.NQuad, 0, len(gmu.Del))
	for _, nq := range gmu.Del {
		// if Subject or/and Object are variables, each NQuad can result
		// in multiple NQuads if any variable stores more than one UIDs.
		newSubs := getNewVals(nq.Subject)
		newObs := getNewVals(nq.ObjectId)

		for _, s := range newSubs {
			for _, o := range newObs {
				// Blank node has no meaning in case of deletion.
				if strings.HasPrefix(s, "_:uid(") ||
					strings.HasPrefix(o, "_:uid(") {
					continue
				}

				gmuDel = append(gmuDel, getNewNQuad(nq, s, o))
			}
		}
	}
	gmu.Del = gmuDel

	// Update the values in mutation block from the query block.
	gmuSet := make([]*api.NQuad, 0, len(gmu.Set))
	for _, nq := range gmu.Set {
		newSubs := getNewVals(nq.Subject)
		newObs := getNewVals(nq.ObjectId)

		for _, s := range newSubs {
			for _, o := range newObs {
				gmuSet = append(gmuSet, getNewNQuad(nq, s, o))
			}
		}
	}
	gmu.Set = gmuSet
}

// queryContext is used to pass around all the variables needed
// to process a request for query, mutation or upsert.
type queryContext struct {
	// req is the incoming, not yet parsed request containing
	// a query or more than one mutations or both (in case of upsert)
	req *api.Request
	// gmuList is the list of mutations after parsing req.Mutations
	gmuList []*gql.Mutation
	// gqlRes contains result of parsing the req.Query
	gqlRes gql.Result
	// condVars are conditional variables used in the (modified) query to figure out
	// whether the condition in Conditional Upsert is true. The string would be empty
	// if the corresponding mutation is not a conditional upsert.
	// Note that, len(condVars) == len(gmuList).
	condVars []string
	// uidRes stores mapping from variable names to UIDs for UID variables.
	// These variables are either dummy variables used for Conditional
	// Upsert or variables used in the mutation block in the incoming request.
	uidRes map[string][]string
	// valRes stores mapping from variable names to values for value
	// variables used in the mutation block of incoming request.
	valRes map[string]map[uint64]types.Val
	// l stores latency numbers
	latency *query.Latency
	// span stores a opencensus span used throughout the query processing
	span *trace.Span
}

// Query handles queries or mutations
func (s *Server) Query(ctx context.Context, req *api.Request) (*api.Response, error) {
	return s.doQuery(ctx, req, NeedAuthorize)
}

func (s *Server) doQuery(ctx context.Context, req *api.Request, authorize int) (
	resp *api.Response, rerr error) {

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	l := &query.Latency{}
	l.Start = time.Now()

	isMutation := len(req.Mutations) > 0
	methodRequest := methodQuery
	if isMutation {
		methodRequest = methodMutate
	}

	var measurements []ostats.Measurement
	ctx, span := otrace.StartSpan(ctx, methodRequest)
	ctx = x.WithMethod(ctx, methodRequest)
	defer func() {
		span.End()
		v := x.TagValueStatusOK
		if rerr != nil {
			v = x.TagValueStatusError
		}
		ctx, _ = tag.New(ctx, tag.Upsert(x.KeyStatus, v))
		timeSpentMs := x.SinceMs(l.Start)
		measurements = append(measurements, x.LatencyMs.M(timeSpentMs))
		ostats.Record(ctx, measurements...)
	}()

	if rerr = x.HealthCheck(); rerr != nil {
		return
	}

	req.Query = strings.TrimSpace(req.Query)
	isQuery := len(req.Query) != 0
	if !isQuery && !isMutation {
		span.Annotate(nil, "empty request")
		return nil, errors.Errorf("empty request")
	}

	span.Annotatef(nil, "Request received: %v", req)
	if isQuery {
		ostats.Record(ctx, x.PendingQueries.M(1), x.NumQueries.M(1))
		defer func() {
			measurements = append(measurements, x.PendingQueries.M(-1))
		}()
	}
	if isMutation {
		ostats.Record(ctx, x.NumMutations.M(1))
	}

	qc := &queryContext{req: req, latency: l, span: span}
	if rerr = parseRequest(qc); rerr != nil {
		return
	}

	if authorize == NeedAuthorize {
		if rerr = authorizeRequest(ctx, qc); rerr != nil {
			return
		}
	}

	// We use defer here because for queries, startTs will be
	// assigned in the processQuery function called below.
	defer annotateStartTs(qc.span, qc.req.StartTs)
	// For mutations, we update the startTs if necessary.
	if isMutation && req.StartTs == 0 {
		start := time.Now()
		req.StartTs = State.getTimestamp(false)
		qc.latency.AssignTimestamp = time.Since(start)
	}

	if resp, rerr = processQuery(ctx, qc); rerr != nil {
		return
	}
	if rerr = s.doMutate(ctx, qc, resp); rerr != nil {
		return
	}

	// TODO(martinmr): Include Transport as part of the latency. Need to do
	// this separately since it involves modifying the API protos.
	resp.Latency = &api.Latency{
		AssignTimestampNs: uint64(l.AssignTimestamp.Nanoseconds()),
		ParsingNs:         uint64(l.Parsing.Nanoseconds()),
		ProcessingNs:      uint64(l.Processing.Nanoseconds()),
		EncodingNs:        uint64(l.Json.Nanoseconds()),
	}

	return resp, nil
}

func processQuery(ctx context.Context, qc *queryContext) (*api.Response, error) {
	resp := &api.Response{}
	if len(qc.req.Query) == 0 {
		return resp, nil
	}

	qr := query.Request{
		Latency:  qc.latency,
		GqlQuery: &qc.gqlRes,
	}

	// Here we try our best effort to not contact Zero for a timestamp. If we succeed,
	// then we use the max known transaction ts value (from ProcessDelta) for a read-only query.
	// If we haven't processed any updates yet then fall back to getting TS from Zero.
	switch {
	case qc.req.BestEffort:
		qc.span.Annotate([]otrace.Attribute{otrace.BoolAttribute("be", true)}, "")
	case qc.req.ReadOnly:
		qc.span.Annotate([]otrace.Attribute{otrace.BoolAttribute("ro", true)}, "")
	default:
		qc.span.Annotate([]otrace.Attribute{otrace.BoolAttribute("no", true)}, "")
	}

	if qc.req.BestEffort {
		// Sanity: check that request is read-only too.
		if !qc.req.ReadOnly {
			return resp, errors.Errorf("A best effort query must be read-only.")
		}
		if qc.req.StartTs == 0 {
			qc.req.StartTs = posting.Oracle().MaxAssigned()
		}
		qr.Cache = worker.NoCache
	}

	if qc.req.StartTs == 0 {
		assignTimestampStart := time.Now()
		qc.req.StartTs = State.getTimestamp(qc.req.ReadOnly)
		qc.latency.AssignTimestamp = time.Since(assignTimestampStart)
	}

	qr.ReadTs = qc.req.StartTs
	resp.Txn = &api.TxnContext{StartTs: qc.req.StartTs}

	// Core processing happens here.
	er, err := qr.Process(ctx)
	if err != nil {
		return resp, errors.Wrap(err, "")
	}

	if len(er.SchemaNode) > 0 || len(er.Types) > 0 {
		sort.Slice(er.SchemaNode, func(i, j int) bool {
			return er.SchemaNode[i].Predicate < er.SchemaNode[j].Predicate
		})
		sort.Slice(er.Types, func(i, j int) bool {
			return er.Types[i].TypeName < er.Types[j].TypeName
		})

		respMap := make(map[string]interface{})
		if len(er.SchemaNode) > 0 {
			respMap["schema"] = er.SchemaNode
		}
		if len(er.Types) > 0 {
			respMap["types"] = formatTypes(er.Types)
		}
		resp.Json, err = json.Marshal(respMap)
	} else {
		resp.Json, err = query.ToJson(qc.latency, er.Subgraphs)
	}
	if err != nil {
		return resp, err
	}
	qc.span.Annotatef(nil, "Response = %s", resp.Json)

	// varToUID contains a map of variable name to the uids corresponding to it.
	// It is used later for constructing set and delete mutations by replacing
	// variables with the actual uids they correspond to.
	// If a variable doesn't have any UID, we generate one ourselves later.
	for name := range qc.uidRes {
		v := qr.Vars[name]
		if v.Uids == nil || len(v.Uids.Uids) <= 0 {
			continue
		}

		uids := make([]string, len(v.Uids.Uids))
		for i, u := range v.Uids.Uids {
			// We use base 10 here because the RDF mutations expect the uid to be in base 10.
			uids[i] = strconv.FormatUint(u, 10)
		}
		qc.uidRes[name] = uids
	}

	// look for values for value variables
	for name := range qc.valRes {
		v := qr.Vars[name]
		qc.valRes[name] = v.Vals
	}

	resp.Metrics = &api.Metrics{
		NumUids: er.Metrics,
	}

	return resp, err
}

// parseRequest parses the incoming request
func parseRequest(qc *queryContext) error {
	start := time.Now()
	defer func() {
		qc.latency.Parsing = time.Since(start)
	}()

	var needVars []string
	upsertQuery := qc.req.Query
	if len(qc.req.Mutations) > 0 {
		// parsing mutations
		qc.gmuList = make([]*gql.Mutation, 0, len(qc.req.Mutations))
		for _, mu := range qc.req.Mutations {
			gmu, err := parseMutationObject(mu)
			if err != nil {
				return err
			}

			qc.gmuList = append(qc.gmuList, gmu)
		}

		qc.uidRes = make(map[string][]string)
		qc.valRes = make(map[string]map[uint64]types.Val)
		upsertQuery = buildUpsertQuery(qc)
		needVars = findMutationVars(qc)
		if len(upsertQuery) == 0 {
			if len(needVars) > 0 {
				return errors.Errorf("variables %v not defined", needVars)
			}

			return nil
		}
	}

	// parsing the updated query
	var err error
	qc.gqlRes, err = gql.ParseWithNeedVars(gql.Request{
		Str:       upsertQuery,
		Variables: qc.req.Vars,
	}, needVars)
	if err != nil {
		return err
	}
	if err = validateQuery(qc.gqlRes.Query); err != nil {
		return err
	}

	return nil
}

func authorizeRequest(ctx context.Context, qc *queryContext) error {
	if err := authorizeQuery(ctx, &qc.gqlRes); err != nil {
		return err
	}

	// TODO(Aman): can be optimized to do the authorization in just one func call
	for _, gmu := range qc.gmuList {
		if err := authorizeMutation(ctx, gmu); err != nil {
			return err
		}
	}

	return nil
}

// CommitOrAbort commits or aborts a transaction.
func (s *Server) CommitOrAbort(ctx context.Context, tc *api.TxnContext) (*api.TxnContext, error) {
	ctx, span := otrace.StartSpan(ctx, "Server.CommitOrAbort")
	defer span.End()

	if err := x.HealthCheck(); err != nil {
		return &api.TxnContext{}, err
	}

	tctx := &api.TxnContext{}
	if tc.StartTs == 0 {
		return &api.TxnContext{}, errors.Errorf(
			"StartTs cannot be zero while committing a transaction")
	}
	annotateStartTs(span, tc.StartTs)

	span.Annotatef(nil, "Txn Context received: %+v", tc)
	commitTs, err := worker.CommitOverNetwork(ctx, tc)
	if err == dgo.ErrAborted {
		tctx.Aborted = true
		return tctx, status.Errorf(codes.Aborted, err.Error())
	}
	tctx.StartTs = tc.StartTs
	tctx.CommitTs = commitTs
	return tctx, err
}

// CheckVersion returns the version of this Dgraph instance.
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

var errNoAuth = errors.Errorf("No Auth Token found. Token needed for Alter operations.")

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
		return errors.Errorf("Provided auth token [%s] does not match. Permission denied.", tokens[0])
	}
	return nil
}

// parseMutationObject tries to consolidate fields of the api.Mutation into the
// corresponding field of the returned gql.Mutation. For example, the 3 fields,
// api.Mutation#SetJson, api.Mutation#SetNquads and api.Mutation#Set are consolidated into the
// gql.Mutation.Set field. Similarly the 3 fields api.Mutation#DeleteJson, api.Mutation#DelNquads
// and api.Mutation#Del are merged into the gql.Mutation#Del field.
func parseMutationObject(mu *api.Mutation) (*gql.Mutation, error) {
	res := &gql.Mutation{Cond: mu.Cond}

	if len(mu.SetJson) > 0 {
		nqs, err := chunker.ParseJSON(mu.SetJson, chunker.SetNquads)
		if err != nil {
			return nil, err
		}
		res.Set = append(res.Set, nqs...)
	}
	if len(mu.DeleteJson) > 0 {
		nqs, err := chunker.ParseJSON(mu.DeleteJson, chunker.DeleteNquads)
		if err != nil {
			return nil, err
		}
		res.Del = append(res.Del, nqs...)
	}
	if len(mu.SetNquads) > 0 {
		nqs, err := chunker.ParseRDFs(mu.SetNquads)
		if err != nil {
			return nil, err
		}
		res.Set = append(res.Set, nqs...)
	}
	if len(mu.DelNquads) > 0 {
		nqs, err := chunker.ParseRDFs(mu.DelNquads)
		if err != nil {
			return nil, err
		}
		res.Del = append(res.Del, nqs...)
	}

	res.Set = append(res.Set, mu.Set...)
	res.Del = append(res.Del, mu.Del...)
	// parse facets and convert to the binary format so that
	// a field of type datetime like "2017-01-01" can be correctly encoded in the
	// marshaled binary format as done in the time.Marshal method
	if err := validateAndConvertFacets(res.Set); err != nil {
		return nil, err
	}

	if err := validateNQuads(res.Set, res.Del); err != nil {
		return nil, err
	}
	return res, nil
}

func validateAndConvertFacets(nquads []*api.NQuad) error {
	for _, m := range nquads {
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
		if err := validatePredName(nq.Predicate); err != nil {
			return err
		}
		var ostar bool
		if o, ok := nq.ObjectValue.GetVal().(*api.Value_DefaultVal); ok {
			ostar = o.DefaultVal == x.Star
		}
		if nq.Subject == x.Star || nq.Predicate == x.Star || ostar {
			return errors.Errorf("Cannot use star in set n-quad: %+v", nq)
		}
		if err := validateKeys(nq); err != nil {
			return errors.Wrapf(err, "key error: %+v", nq)
		}
	}
	for _, nq := range del {
		if err := validatePredName(nq.Predicate); err != nil {
			return err
		}
		var ostar bool
		if o, ok := nq.ObjectValue.GetVal().(*api.Value_DefaultVal); ok {
			ostar = o.DefaultVal == x.Star
		}
		if nq.Subject == x.Star || (nq.Predicate == x.Star && !ostar) {
			return errors.Errorf("Only valid wildcard delete patterns are 'S * *' and 'S P *': %v", nq)
		}
		// NOTE: we dont validateKeys() with delete to let users fix existing mistakes
		// with bad predicate forms. ex: foo@bar ~something
	}
	return nil
}

func validateKey(key string) error {
	switch {
	case len(key) == 0:
		return errors.Errorf("Has zero length")
	case strings.ContainsAny(key, "~@"):
		return errors.Errorf("Has invalid characters")
	case strings.IndexFunc(key, unicode.IsSpace) != -1:
		return errors.Errorf("Must not contain spaces")
	}
	return nil
}

// validateKeys checks predicate and facet keys in N-Quad for syntax errors.
func validateKeys(nq *api.NQuad) error {
	if err := validateKey(nq.Predicate); err != nil {
		return errors.Wrapf(err, "predicate %q", nq.Predicate)
	}
	for i := range nq.Facets {
		if nq.Facets[i] == nil {
			continue
		}
		if err := validateKey(nq.Facets[i].Key); err != nil {
			return errors.Errorf("Facet %q, %s", nq.Facets[i].Key, err)
		}
	}
	return nil
}

// validateQuery verifies that the query does not contain any preds that
// are longer than the limit (2^16).
func validateQuery(queries []*gql.GraphQuery) error {
	for _, q := range queries {
		// These are used in the response of a mutation in HTTP API.
		if a := strings.ToLower(q.Alias); a == "code" || a == "message" || a == "uids" {
			return errors.Errorf("query alias [%v] not allowed", a)
		}

		if err := validatePredName(q.Attr); err != nil {
			return err
		}

		if err := validateQuery(q.Children); err != nil {
			return err
		}
	}

	return nil
}

func validatePredName(name string) error {
	if len(name) > math.MaxUint16 {
		return errors.Errorf("Predicate name length cannot be bigger than 2^16. Predicate: %v",
			name[:80])
	}
	return nil
}

// formatTypes takes a list of TypeUpdates and converts them in to a list of
// maps in a format that is human-readable to be marshaled into JSON.
func formatTypes(types []*pb.TypeUpdate) []map[string]interface{} {
	var res []map[string]interface{}
	for _, typ := range types {
		typeMap := make(map[string]interface{})
		typeMap["name"] = typ.TypeName
		fields := make([]map[string]string, len(typ.Fields))

		for i, field := range typ.Fields {
			m := make(map[string]string, 1)
			m["name"] = field.Predicate
			fields[i] = m
		}
		typeMap["fields"] = fields

		res = append(res, typeMap)
	}
	return res
}

func isDropAll(op *api.Operation) bool {
	if op.DropAll || op.DropOp == api.Operation_ALL {
		return true
	}
	return false
}
