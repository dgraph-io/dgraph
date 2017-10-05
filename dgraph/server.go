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
package dgraph

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/net/trace"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"
)

type ServerState struct {
	FinishCh   chan struct{} // channel to wait for all pending reqs to finish.
	ShutdownCh chan struct{} // channel to signal shutdown.

	Pstore   *badger.KV
	WALstore *badger.KV

	vlogTicker *time.Ticker
}

// TODO(tzdybal) - remove global
var State ServerState

func NewServerState() (state ServerState) {
	Config.validate()

	state.FinishCh = make(chan struct{})
	state.ShutdownCh = make(chan struct{})

	state.initStorage()

	return state
}

func (s *ServerState) runVlogGC(store *badger.KV) {
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
	s.WALstore, err = badger.NewKV(&kvOpt)
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
	s.Pstore, err = badger.NewKV(&opt)
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

// This method is used to execute the query and return the response to the
// client as a protocol buffer message.
func (s *Server) Run(ctx context.Context, req *protos.Request) (resp *protos.Response, err error) {
	// we need membership information
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

	// Sanitize the context of the keys used for internal purposes only
	ctx = context.WithValue(ctx, "_share_", nil)
	ctx = context.WithValue(ctx, "mutation_allowed", isMutationAllowed(ctx))

	resp = new(protos.Response)
	emptyMutation := len(req.Mutation.GetSet()) == 0 && len(req.Mutation.GetDel()) == 0 &&
		len(req.Mutation.GetSchema()) == 0 && len(req.Mutation.GetSetJson()) == 0 &&
		len(req.Mutation.GetDeleteJson()) == 0 && !req.Mutation.GetDropAll()
	if len(req.Query) == 0 && emptyMutation && req.Schema == nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Empty query and mutation.")
		}
		return resp, fmt.Errorf("empty query and mutation.")
	}

	if Config.DebugMode {
		x.Printf("Received query: %+v, mutation: %+v\n", req.Query, req.Mutation)
	}
	var l query.Latency
	l.Start = time.Now()
	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("Query received: %v, variables: %v", req.Query, req.Vars)
	}

	res, err := gql.Parse(gql.Request{
		Str:       req.Query,
		Mutation:  req.Mutation,
		Variables: req.Vars,
		Http:      false,
	})
	if err != nil {
		return resp, err
	}

	if err := parseMutationObject(&res, req); err != nil {
		return resp, err
	}

	var cancel context.CancelFunc
	// set timeout if schema mutation not present
	if res.Mutation == nil || len(res.Mutation.Schema) == 0 {
		// If schema mutation is not present
		ctx, cancel = context.WithTimeout(ctx, time.Minute)
		defer cancel()
	}

	if req.Schema != nil && res.Schema != nil {
		return resp, x.Errorf("Multiple schema blocks found")
	}
	// Schema Block and Mutation can be part of query string or request
	if res.Schema == nil {
		res.Schema = req.Schema
	}

	var queryRequest = query.QueryRequest{
		Latency:  &l,
		GqlQuery: &res,
	}
	if req.Mutation != nil && len(req.Mutation.Schema) > 0 {
		// Every update that comes from the client is explicit.
		for _, s := range req.Mutation.Schema {
			s.Explicit = true
		}
		queryRequest.SchemaUpdate = req.Mutation.Schema
	}

	var er query.ExecuteResult
	if er, err = queryRequest.ProcessWithMutation(ctx); err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Error while processing query: %+v", err)
		}
		return resp, x.Wrap(err)
	}
	resp.AssignedUids = er.Allocations
	resp.Schema = er.SchemaNode

	nodes, err := query.ToProtocolBuf(&l, er.Subgraphs)
	if err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Error while converting to protocol buffer: %+v", err)
		}
		return resp, err
	}
	resp.N = nodes

	gl := new(protos.Latency)
	gl.Parsing, gl.Processing, gl.Pb = l.Parsing.String(), l.Processing.String(),
		l.ProtocolBuffer.String()
	resp.L = gl
	return resp, err
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

func (s *Server) AssignUids(ctx context.Context, num *protos.Num) (*protos.AssignedIds, error) {
	if err := x.HealthCheck(); err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("request rejected %v", err)
		}
		return &protos.AssignedIds{}, err
	}
	return worker.AssignUidsOverNetwork(ctx, num)
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

func parseFacets(val interface{}) ([]*protos.Facet, error) {
	if val == nil {
		return nil, nil
	}

	facetObj, ok := val.(map[string]interface{})
	if !ok {
		return nil, x.Errorf("Facets : %v should always be a map", val)
	}

	facetsForPred := make([]*protos.Facet, 0, len(facetObj))
	var fv interface{}
	for facetKey, facetVal := range facetObj {
		if facetVal == nil {
			continue
		}
		f := &protos.Facet{Key: facetKey}
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
				facetKey)
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

		var g geom.T
		err := geojson.Unmarshal([]byte(v.(string)), &g)
		// We try to parse the value as a GeoJSON. If we can't, then we store as a string.
		if err == nil {
			geo, err := types.ObjectValue(types.GeoID, g)
			if err != nil {
				return x.Errorf("Couldn't convert value: %s to geo type", v.(string))
			}

			nq.ObjectValue = geo
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

func mapToNquads(m map[string]interface{}, idx *int, op int) (mapResponse, error) {
	var mr mapResponse
	// Check field in map.
	if uidVal, ok := m["_uid_"]; ok {
		// Should be convertible to uint64. Maybe we also want to allow string later.
		if id, ok := uidVal.(float64); ok && uint64(id) != 0 {
			mr.uid = fmt.Sprintf("%d", uint64(id))
		}
	}

	if len(mr.uid) == 0 && op != delete {
		mr.uid = fmt.Sprintf("_:blank-%d", *idx)
		*idx++
	}

	// Since _uid_ is the only key, this must be S * * deletion.
	if op == delete && len(mr.uid) > 0 && len(m) == 1 {
		mr.nquads = append(mr.nquads, &protos.NQuad{
			Subject:     mr.uid,
			Predicate:   x.Star,
			ObjectValue: &protos.Value{&protos.Value_DefaultVal{x.Star}},
		})
		return mr, nil
	}

	for k, v := range m {
		// We have already extracted the uid above so we skip that edge.
		// v can be nil if user didn't set a value and if omitEmpty was not supplied as JSON
		// option.
		// We also skip facets here because we parse them with the corresponding predicate.
		if k == "_uid_" || strings.HasSuffix(k, "@facets") {
			continue
		}

		if op == delete {
			// This corresponds to predicate deletion.
			if v == nil {
				mr.nquads = append(mr.nquads, &protos.NQuad{
					Subject:     x.Star,
					Predicate:   k,
					ObjectValue: &protos.Value{&protos.Value_DefaultVal{x.Star}},
				})
				continue
			} else if len(mr.uid) == 0 {
				// Delete operations with a non-nil value must have a uid specified.
				return mr, x.Errorf("_uid_ must be present and non-zero. Got: %+v", m)
			}
		}

		fkey := fmt.Sprintf("%s@facets", k)
		fts, err := parseFacets(m[fkey])
		if err != nil {
			return mr, err
		}

		nq := protos.NQuad{
			Subject:   mr.uid,
			Predicate: k,
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
			if err := handleBasicType(k, v, op, &nq); err != nil {
				return mr, err
			}
			mr.nquads = append(mr.nquads, &nq)
		case map[string]interface{}:
			cr, err := mapToNquads(v.(map[string]interface{}), idx, op)
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
					Predicate: k,
				}

				switch iv := item.(type) {
				case string, float64:
					if err := handleBasicType(k, iv, op, &nq); err != nil {
						return mr, err
					}
					mr.nquads = append(mr.nquads, &nq)
				case map[string]interface{}:
					cr, err := mapToNquads(iv, idx, op)
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
						x.Errorf("Got unsupported type for list: %s", k)
				}
			}
		default:
			return mr, x.Errorf("Unexpected type for val for attr: %s while converting to nquad", k)
		}
	}

	fts, err := parseFacets(m["@facets"])
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
			mr, err := mapToNquads(obj.(map[string]interface{}), &idx, op)
			if err != nil {
				return mr.nquads, err
			}

			nquads = append(nquads, mr.nquads...)
		}
		return nquads, nil
	}

	mr, err := mapToNquads(ms, &idx, op)
	return mr.nquads, err
}

func parseMutationObject(res *gql.Result, q *protos.Request) error {
	if q.Mutation == nil || (len(q.Mutation.SetJson) == 0 && len(q.Mutation.DeleteJson) == 0) {
		return nil
	}

	var nquads []*protos.NQuad
	var err error
	if len(q.Mutation.SetJson) > 0 {
		nquads, err = nquadsFromJson(q.Mutation.SetJson, set)
		if err != nil {
			return err
		}
	}

	if res.Mutation == nil {
		res.Mutation = &gql.Mutation{}
	}
	res.Mutation.Set = append(res.Mutation.Set, nquads...)

	nquads = nquads[:0]
	if len(q.Mutation.DeleteJson) > 0 {
		nquads, err = nquadsFromJson(q.Mutation.DeleteJson, delete)
		if err != nil {
			return err
		}
	}
	res.Mutation.Del = append(res.Mutation.Del, nquads...)

	return nil
}
