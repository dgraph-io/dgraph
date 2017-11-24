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
package edgraph

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"golang.org/x/net/context"
	"golang.org/x/net/trace"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/rdf"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/dgraph/y"
	"github.com/pkg/errors"
	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"
)

type ServerState struct {
	FinishCh   chan struct{} // channel to wait for all pending reqs to finish.
	ShutdownCh chan struct{} // channel to signal shutdown.

	Pstore   *badger.ManagedDB
	WALstore *badger.ManagedDB

	vlogTicker *time.Ticker

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
	// TODO - Make this smarter later. Maybe get size of directories from badger and only run GC
	// if size increases by more than 1GB.
	for range s.vlogTicker.C {
		store.RunValueLogGC(0.5)
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
	s.WALstore, err = badger.OpenManaged(kvOpt)
	x.Checkf(err, "Error while creating badger KV WAL store")

	// Postings directory
	// All the writes to posting store should be synchronous. We use batched writers
	// for posting lists, so the cost of sync writes is amortized.
	x.Check(os.MkdirAll(Config.PostingDir, 0700))
	opt := badger.DefaultOptions
	opt.SyncWrites = true
	opt.Dir = Config.PostingDir
	opt.ValueDir = Config.PostingDir
	switch Config.PostingTables {
	case "memorymap":
		opt.TableLoadingMode = options.MemoryMap
	case "loadtoram":
		opt.TableLoadingMode = options.LoadToRAM
	case "fileio":
		opt.TableLoadingMode = options.FileIO
	default:
		x.Fatalf("Invalid Posting Tables options")
	}
	s.Pstore, err = badger.OpenManaged(opt)
	x.Checkf(err, "Error while creating badger KV posting store")
	s.vlogTicker = time.NewTicker(10 * time.Minute)
	go s.runVlogGC(s.Pstore)
	go s.runVlogGC(s.WALstore)
}

func (s *ServerState) Dispose() error {
	if err := s.Pstore.Close(); err != nil {
		return errors.Wrapf(err, "While closing postings store")
	}
	if err := s.WALstore.Close(); err != nil {
		return errors.Wrapf(err, "While closing WAL store")
	}
	s.vlogTicker.Stop()
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
		num := &protos.Num{Val: uint64(len(chs))}
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

func (s *Server) Alter(ctx context.Context, op *protos.Operation) (*protos.Payload, error) {
	empty := &protos.Payload{}
	if err := x.HealthCheck(); err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Request rejected %v", err)
		}
		return empty, err
	}

	if op.DropAll {
		m := protos.Mutations{DropAll: true}
		_, err := query.ApplyMutations(ctx, &m)
		return empty, err
	}
	if len(op.DropAttr) > 0 {
		nq := &protos.NQuad{
			Subject:     x.Star,
			Predicate:   op.DropAttr,
			ObjectValue: &protos.Value{&protos.Value_StrVal{x.Star}},
		}
		wnq := &gql.NQuad{nq}
		edge, err := wnq.ToDeletePredEdge()
		if err != nil {
			return empty, err
		}
		edges := []*protos.DirectedEdge{edge}
		m := &protos.Mutations{Edges: edges}
		_, err = query.ApplyMutations(ctx, m)
		return empty, err
	}
	updates, err := schema.Parse(op.Schema)
	if err != nil {
		return empty, err
	}
	for _, u := range updates {
		u.Explicit = true
	}
	fmt.Printf("Got schema: %+v\n", updates)
	// TODO: Maybe add some checks about the schema.
	if op.StartTs == 0 {
		op.StartTs = State.getTimestamp()
	}
	m := &protos.Mutations{Schema: updates, StartTs: op.StartTs}
	_, err = query.ApplyMutations(ctx, m)
	return empty, err
}

func (s *Server) Mutate(ctx context.Context, mu *protos.Mutation) (resp *protos.Assigned, err error) {
	resp = &protos.Assigned{}
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
		tr, ctx = x.NewTrace("GrpcMutate", ctx)
		defer tr.Finish()
	}
	gmu, err := parseMutationObject(mu)
	if err != nil {
		return resp, err
	}
	newUids, err := query.AssignUids(ctx, gmu.Set)
	if err != nil {
		return resp, err
	}
	resp.Uids = query.ConvertUidsToHex(query.StripBlankNode(newUids))
	edges, err := query.ToInternal(gmu, newUids)
	if err != nil {
		return resp, err
	}

	m := &protos.Mutations{Edges: edges, StartTs: mu.StartTs}
	resp.Context, err = query.ApplyMutations(ctx, m)
	if !mu.CommitNow {
		if err == y.ErrConflict {
			err = status.Errorf(codes.FailedPrecondition, err.Error())
		}
		return resp, err
	}

	// The following logic is for committing immediately.
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
	resp.Context.CommitTs = cts
	return resp, nil
}

// This method is used to execute the query and return the response to the
// client as a protocol buffer message.
func (s *Server) Query(ctx context.Context, req *protos.Request) (resp *protos.Response, err error) {
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

	resp = new(protos.Response)
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
		Http:      false,
	})
	if err != nil {
		return resp, err
	}

	if req.StartTs == 0 {
		req.StartTs = State.getTimestamp()
	}
	resp.Txn = &protos.TxnContext{
		StartTs: req.StartTs,
	}

	var queryRequest = query.QueryRequest{
		Latency:  &l,
		GqlQuery: &parsedReq,
		ReadTs:   req.StartTs,
		LinRead:  req.LinRead,
	}

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

	gl := &protos.Latency{
		ParsingNs:    uint64(l.Parsing.Nanoseconds()),
		ProcessingNs: uint64(l.Processing.Nanoseconds()),
		EncodingNs:   uint64(l.Json.Nanoseconds()),
	}

	resp.Latency = gl
	resp.Txn.LinRead = queryRequest.LinRead
	return resp, err
}

func (s *Server) CommitOrAbort(ctx context.Context, tc *protos.TxnContext) (*protos.TxnContext,
	error) {
	if err := x.HealthCheck(); err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Request rejected %v", err)
		}
		return &protos.TxnContext{}, err
	}

	tctx := &protos.TxnContext{}

	commitTs, err := worker.CommitOverNetwork(ctx, tc)
	if err == y.ErrAborted {
		tctx.Aborted = true
		return tctx, status.Errorf(codes.Aborted, err.Error())
	}
	tctx.CommitTs = commitTs
	return tctx, err
}

func (s *Server) CheckVersion(ctx context.Context, c *protos.Check) (v *protos.Version, err error) {
	if err := x.HealthCheck(); err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("request rejected %v", err)
		}
		return v, err
	}

	v = new(protos.Version)
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

func parseFacets(m map[string]interface{}, prefix string) ([]*protos.Facet, error) {
	// This happens at root.
	if prefix == "" {
		return nil, nil
	}

	var facetsForPred []*protos.Facet
	var fv interface{}
	for fname, facetVal := range m {
		if facetVal == nil {
			continue
		}
		if !strings.HasPrefix(fname, prefix) {
			continue
		}

		if len(fname) <= len(prefix) {
			return nil, x.Errorf("Facet key is invalid: %s", fname)
		}
		// Prefix includes colon, predicate:
		f := &protos.Facet{Key: fname[len(prefix):]}
		switch v := facetVal.(type) {
		case string:
			if t, err := types.ParseTime(v); err == nil {
				f.ValType = protos.Facet_DATETIME
				fv = t
			} else {
				f.ValType = protos.Facet_STRING
				fv = v
			}
		case float64:
			// Could be int too, but we just store it as float.
			fv = v
			f.ValType = protos.Facet_FLOAT
		case bool:
			fv = v
			f.ValType = protos.Facet_BOOL
		default:
			return nil, x.Errorf("Facet value for key: %s can only be string/float64/bool.",
				fname)
		}

		// convert facet val interface{} to binary
		tid := facets.TypeIDFor(&protos.Facet{ValType: f.ValType})
		fVal := &types.Val{Tid: types.BinaryID}
		if err := types.Marshal(types.Val{Tid: tid, Value: fv}, fVal); err != nil {
			return nil, err
		}

		fval, ok := fVal.Value.([]byte)
		if !ok {
			return nil, x.Errorf("Error while marshalling types.Val into binary.")
		}
		f.Value = fval
		facetsForPred = append(facetsForPred, f)
	}

	return facetsForPred, nil
}

// This is the response for a map[string]interface{} i.e. a struct.
type mapResponse struct {
	nquads []*protos.NQuad // nquads at this level including the children.
	uid    string          // uid retrieved or allocated for the node.
	fcts   []*protos.Facet // facets on the edge connecting this node to the source if any.
}

func handleBasicType(k string, v interface{}, op int, nq *protos.NQuad) error {
	switch v.(type) {
	case string:
		predWithLang := strings.SplitN(k, "@", 2)
		if len(predWithLang) == 2 && predWithLang[0] != "" {
			nq.Predicate = predWithLang[0]
			nq.Lang = predWithLang[1]
		}

		// Default value is considered as S P * deletion.
		if v == "" && op == delete {
			nq.ObjectValue = &protos.Value{&protos.Value_DefaultVal{x.Star}}
			return nil
		}

		nq.ObjectValue = &protos.Value{&protos.Value_StrVal{v.(string)}}
	case float64:
		if v == 0 && op == delete {
			nq.ObjectValue = &protos.Value{&protos.Value_DefaultVal{x.Star}}
			return nil
		}

		nq.ObjectValue = &protos.Value{&protos.Value_DoubleVal{v.(float64)}}
	case bool:
		if v == false && op == delete {
			nq.ObjectValue = &protos.Value{&protos.Value_DefaultVal{x.Star}}
			return nil
		}

		nq.ObjectValue = &protos.Value{&protos.Value_BoolVal{v.(bool)}}
	default:
		return x.Errorf("Unexpected type for val for attr: %s while converting to nquad", k)
	}
	return nil

}

func checkForDeletion(mr *mapResponse, m map[string]interface{}, op int) {
	// Since uid is the only key, this must be S * * deletion.
	if op == delete && len(mr.uid) > 0 && len(m) == 1 {
		mr.nquads = append(mr.nquads, &protos.NQuad{
			Subject:     mr.uid,
			Predicate:   x.Star,
			ObjectValue: &protos.Value{&protos.Value_DefaultVal{x.Star}},
		})
	}
}

func tryParseAsGeo(b []byte, nq *protos.NQuad) (bool, error) {
	var g geom.T
	err := geojson.Unmarshal(b, &g)
	if err == nil {
		geo, err := types.ObjectValue(types.GeoID, g)
		if err != nil {
			return false, x.Errorf("Couldn't convert value: %s to geo type", string(b))
		}

		nq.ObjectValue = geo
		return true, nil
	}
	return false, nil
}

// TODO - Abstract these parameters to a struct.
func mapToNquads(m map[string]interface{}, idx *int, op int, parentPred string) (mapResponse, error) {
	var mr mapResponse
	// Check field in map.
	if uidVal, ok := m["uid"]; ok {
		var uid uint64
		if id, ok := uidVal.(float64); ok {
			uid = uint64(id)
		} else if id, ok := uidVal.(string); ok {
			if u, err := strconv.ParseInt(id, 0, 64); err == nil {
				uid = uint64(u)
			}
		}

		if uid > 0 {
			mr.uid = fmt.Sprintf("%d", uid)
		}

	}

	if len(mr.uid) == 0 {
		if op == delete {
			// Delete operations with a non-nil value must have a uid specified.
			return mr, x.Errorf("uid must be present and non-zero while deleting edges.")
		}

		mr.uid = fmt.Sprintf("_:blank-%d", *idx)
		*idx++
	}

	for pred, v := range m {
		// We have already extracted the uid above so we skip that edge.
		// v can be nil if user didn't set a value and if omitEmpty was not supplied as JSON
		// option.
		// We also skip facets here because we parse them with the corresponding predicate.
		if pred == "uid" || strings.Index(pred, query.FacetDelimeter) > 0 {
			continue
		}

		if op == delete {
			// This corresponds to edge deletion.
			if v == nil {
				mr.nquads = append(mr.nquads, &protos.NQuad{
					Subject:     mr.uid,
					Predicate:   pred,
					ObjectValue: &protos.Value{&protos.Value_DefaultVal{x.Star}},
				})
				continue
			}
		}

		prefix := pred + query.FacetDelimeter
		// TODO - Maybe do an initial pass and build facets for all predicates. Then we don't have
		// to call parseFacets everytime.
		fts, err := parseFacets(m, prefix)
		if err != nil {
			return mr, err
		}

		nq := protos.NQuad{
			Subject:   mr.uid,
			Predicate: pred,
			Facets:    fts,
		}

		if v == nil {
			if op == delete {
				nq.ObjectValue = &protos.Value{&protos.Value_DefaultVal{x.Star}}
				mr.nquads = append(mr.nquads, &nq)
			}
			continue
		}

		switch v.(type) {
		case string, float64, bool:
			if err := handleBasicType(pred, v, op, &nq); err != nil {
				return mr, err
			}
			mr.nquads = append(mr.nquads, &nq)
		case map[string]interface{}:
			val := v.(map[string]interface{})
			if len(val) == 0 {
				continue
			}

			// Geojson geometry should have type and coordinates.
			_, hasType := val["type"]
			_, hasCoordinates := val["coordinates"]
			if len(val) == 2 && hasType && hasCoordinates {
				b, err := json.Marshal(val)
				if err != nil {
					return mr, x.Errorf("Error while trying to parse "+
						"value: %+v as geo val", val)
				}
				ok, err := tryParseAsGeo(b, &nq)
				if err != nil {
					return mr, err
				}
				if ok {
					mr.nquads = append(mr.nquads, &nq)
					continue
				}
			}

			cr, err := mapToNquads(v.(map[string]interface{}), idx, op, pred)
			if err != nil {
				return mr, err
			}

			// Add the connecting edge beteween the entities.
			nq.ObjectId = cr.uid
			nq.Facets = cr.fcts
			mr.nquads = append(mr.nquads, &nq)
			// Add the nquads that we got for the connecting entity.
			mr.nquads = append(mr.nquads, cr.nquads...)
		case []interface{}:
			for _, item := range v.([]interface{}) {
				nq := protos.NQuad{
					Subject:   mr.uid,
					Predicate: pred,
				}

				switch iv := item.(type) {
				case string, float64:
					if err := handleBasicType(pred, iv, op, &nq); err != nil {
						return mr, err
					}
					mr.nquads = append(mr.nquads, &nq)
				case map[string]interface{}:
					cr, err := mapToNquads(iv, idx, op, pred)
					if err != nil {
						return mr, err
					}
					nq.ObjectId = cr.uid
					nq.Facets = cr.fcts
					mr.nquads = append(mr.nquads, &nq)
					// Add the nquads that we got for the connecting entity.
					mr.nquads = append(mr.nquads, cr.nquads...)
				default:
					return mr,
						x.Errorf("Got unsupported type for list: %s", pred)
				}
			}
		default:
			return mr, x.Errorf("Unexpected type for val for attr: %s while converting to nquad", pred)
		}
	}

	fts, err := parseFacets(m, parentPred+query.FacetDelimeter)
	mr.fcts = fts
	return mr, err
}

const (
	set = iota
	delete
)

func nquadsFromJson(b []byte, op int) ([]*protos.NQuad, error) {
	ms := make(map[string]interface{})
	var list []interface{}
	if err := json.Unmarshal(b, &ms); err != nil {
		// Couldn't parse as map, lets try to parse it as a list.
		if err = json.Unmarshal(b, &list); err != nil {
			return nil, err
		}
	}

	if len(list) == 0 && len(ms) == 0 {
		return nil, fmt.Errorf("Couldn't parse json as a map or an array.")
	}

	var idx int
	var nquads []*protos.NQuad
	if len(list) > 0 {
		for _, obj := range list {
			if _, ok := obj.(map[string]interface{}); !ok {
				return nil, x.Errorf("Only array of map allowed at root.")
			}
			mr, err := mapToNquads(obj.(map[string]interface{}), &idx, op, "")
			if err != nil {
				return mr.nquads, err
			}
			checkForDeletion(&mr, obj.(map[string]interface{}), op)
			nquads = append(nquads, mr.nquads...)
		}
		return nquads, nil
	}

	mr, err := mapToNquads(ms, &idx, op, "")
	checkForDeletion(&mr, ms, op)
	return mr.nquads, err
}

func parseNQuads(b []byte, op int) ([]*protos.NQuad, error) {
	var nqs []*protos.NQuad
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

func parseMutationObject(mu *protos.Mutation) (*gql.Mutation, error) {
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
		nqs, err := parseNQuads(mu.SetNquads, set)
		if err != nil {
			return nil, err
		}
		res.Set = append(res.Set, nqs...)
	}
	if len(mu.DelNquads) > 0 {
		nqs, err := parseNQuads(mu.DelNquads, delete)
		if err != nil {
			return nil, err
		}
		res.Del = append(res.Del, nqs...)
	}
	res.Set = append(res.Set, mu.Set...)
	res.Del = append(res.Del, mu.Del...)

	return res, validWildcards(res.Set, res.Del)
}

func validWildcards(set, del []*protos.NQuad) error {
	for _, nq := range set {
		var ostar bool
		if o, ok := nq.ObjectValue.GetVal().(*protos.Value_DefaultVal); ok {
			ostar = o.DefaultVal == x.Star
		}
		if nq.Subject == x.Star || nq.Predicate == x.Star || ostar {
			return x.Errorf("Cannot use star in set n-quad: %+v", nq)
		}
	}
	for _, nq := range del {
		var ostar bool
		if o, ok := nq.ObjectValue.GetVal().(*protos.Value_DefaultVal); ok {
			ostar = o.DefaultVal == x.Star
		}
		if nq.Subject == x.Star || (nq.Predicate == x.Star && !ostar) {
			return x.Errorf("Only valid wildcard delete patterns are 'S * *' and 'S P *': %v", nq)
		}
	}
	return nil
}
