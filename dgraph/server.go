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
}

func (s *ServerState) Dispose() error {
	if err := s.Pstore.Close(); err != nil {
		return errors.Wrapf(err, "While closing postings store")
	}
	if err := s.WALstore.Close(); err != nil {
		return errors.Wrapf(err, "While closing WAL store")
	}
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
		len(req.Mutation.GetSchema()) == 0 && len(req.MutationSet) == 0 && len(req.MutationDel) == 0
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

	res, err := ParseQueryAndMutation(ctx, gql.Request{
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

// parseQueryAndMutation handles the cases where the query parsing code can hang indefinitely.
// We allow 1 second for parsing the query; and then give up.
func ParseQueryAndMutation(ctx context.Context, r gql.Request) (res gql.Result, err error) {
	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("Query received: %v", r.Str)
	}
	errc := make(chan error, 1)

	go func() {
		var err error
		res, err = gql.Parse(r)
		errc <- err
	}()

	child, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	select {
	case <-child.Done():
		return res, child.Err()
	case err := <-errc:
		if err != nil {
			if tr, ok := trace.FromContext(ctx); ok {
				tr.LazyPrintf("Error while parsing query: %+v", err)
			}
			return res, err
		}
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Query parsed")
		}
	}
	return res, nil
}

func mapToNquads(m map[string]interface{}, idx *int) ([]*protos.NQuad, string, error) {
	var uid string
	// Check field in map.
	if uidVal, ok := m["_uid_"]; ok {
		// Should be convertible to uint64. Maybe we also want to allow string later.
		if id, ok := uidVal.(float64); ok && uint64(id) != 0 {
			uid = fmt.Sprintf("%d", uint64(id))
		}
	}

	if len(uid) == 0 {
		uid = fmt.Sprintf("_:blank-%d", *idx)
		*idx++
	}

	// TODO - Handle facets
	var nquads []*protos.NQuad
	for k, v := range m {
		// We have already extracted the uid above so we skip that edge.
		// v can be nil if user didn't set a value and if omitEmpty was not supplied as JSON
		// option.
		if k == "_uid_" || v == nil {
			continue
		}

		nq := protos.NQuad{
			Subject:   uid,
			Predicate: k,
		}
		switch v.(type) {
		default:
			return nil, uid, x.Errorf("Unexpected type for val for attr: %s while converting to nquad", k)
		case string:
			predWithLang := strings.SplitN(k, "@", 2)
			if len(predWithLang) == 2 {
				nq.Predicate = predWithLang[0]
				nq.Lang = predWithLang[1]
			}

			var g geom.T
			err := geojson.Unmarshal([]byte(v.(string)), &g)
			// We try to parse the value as a GeoJSON. If we can't, then we store as a string.
			if err == nil {
				geo, err := types.ObjectValue(types.GeoID, g)
				if err != nil {
					return nil, uid, x.Errorf("Couldn't convert value: %s to geo type", v.(string))
				}

				nq.ObjectValue = geo
				nq.ObjectType = int32(types.GeoID)
				nquads = append(nquads, &nq)
				continue
			}

			nq.ObjectValue = &protos.Value{&protos.Value_StrVal{v.(string)}}
			nq.ObjectType = int32(types.StringID)
			nquads = append(nquads, &nq)
		case float64:
			nq.ObjectValue = &protos.Value{&protos.Value_DoubleVal{v.(float64)}}
			nq.ObjectType = int32(types.FloatID)
			nquads = append(nquads, &nq)
		case bool:
			nq.ObjectValue = &protos.Value{&protos.Value_BoolVal{v.(bool)}}
			nq.ObjectType = int32(types.BoolID)
			nquads = append(nquads, &nq)
		case map[string]interface{}:
			// TODO - Handle Geo
			mnquads, oid, err := mapToNquads(v.(map[string]interface{}), idx)
			if err != nil {
				return nil, uid, err
			}

			// Add the connecting edge beteween the entities.
			nq.ObjectId = oid
			nquads = append(nquads, &nq)
			// Add the nquads that we got for the connecting entity.
			nquads = append(nquads, mnquads...)
		case []interface{}:
			for _, item := range v.([]interface{}) {
				nq := protos.NQuad{
					Subject:   uid,
					Predicate: k,
				}

				if mp, ok := item.(map[string]interface{}); ok {
					mnquads, oid, err := mapToNquads(mp, idx)
					if err != nil {
						return nil, uid, err
					}
					nq.ObjectId = oid
					nquads = append(nquads, &nq)
					// Add the nquads that we got for the connecting entity.
					nquads = append(nquads, mnquads...)
				}
			}
		}
	}

	return nquads, uid, nil
}

func nquadsFromJson(b []byte) ([]*protos.NQuad, error) {
	ms := make(map[string]interface{})
	if err := json.Unmarshal(b, &ms); err != nil {
		return nil, err
	}

	var idx int
	nquads, _, err := mapToNquads(ms, &idx)
	return nquads, err
}

func parseMutationObject(res *gql.Result, q *protos.Request) error {
	if len(q.MutationSet) == 0 && len(q.MutationDel) == 0 {
		return nil
	}

	var nquads []*protos.NQuad
	var err error
	if len(q.MutationSet) > 0 {
		nquads, err = nquadsFromJson(q.MutationSet)
		if err != nil {
			return err
		}
	}

	if res.Mutation == nil {
		res.Mutation = &gql.Mutation{}
	}
	res.Mutation.Set = append(res.Mutation.Set, nquads...)

	nquads = nquads[:0]
	if len(q.MutationDel) > 0 {
		nquads, err = nquadsFromJson(q.MutationDel)
		if err != nil {
			return err
		}
	}
	res.Mutation.Del = append(res.Mutation.Del, nquads...)

	return nil
}
