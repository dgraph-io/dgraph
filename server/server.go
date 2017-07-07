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
package server

import (
	"fmt"
	"math/rand"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/net/trace"

	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

type ServerState struct {
	PendingQueries chan struct{}
	FinishCh       chan struct{} // channel to wait for all pending reqs to finish.
	ShutdownCh     chan struct{} // channel to signal shutdown.
}

// TODO(tzdybal) - remove global
var State = NewServerState()

func NewServerState() (state ServerState) {
	state.PendingQueries = make(chan struct{}, *Config.NumPending)
	state.FinishCh = make(chan struct{})
	state.ShutdownCh = make(chan struct{})
	return state
}

// server is used to implement protos.DgraphServer
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
	State.PendingQueries <- struct{}{}
	defer func() { <-State.PendingQueries }()
	if ctx.Err() != nil {
		return resp, ctx.Err()
	}

	if rand.Float64() < *worker.Tracing {
		tr := trace.New("Dgraph", "GrpcQuery")
		tr.SetMaxEvents(1000)
		defer tr.Finish()
		ctx = trace.NewContext(ctx, tr)
	}

	// Sanitize the context of the keys used for internal purposes only
	ctx = context.WithValue(ctx, "_share_", nil)
	ctx = context.WithValue(ctx, "mutation_allowed", isMutationAllowed(ctx))

	resp = new(protos.Response)
	emptyMutation := len(req.Mutation.GetSet()) == 0 && len(req.Mutation.GetDel()) == 0 &&
		len(req.Mutation.GetSchema()) == 0
	if len(req.Query) == 0 && emptyMutation {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Empty query and mutation.")
		}
		return resp, fmt.Errorf("empty query and mutation.")
	}

	var l query.Latency
	l.Start = time.Now()
	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("Query received: %v, variables: %v", req.Query, req.Vars)
	}
	res, err := ParseQueryAndMutation(ctx, gql.Request{
		Str:       req.Query,
		Variables: req.Vars,
		Http:      false,
	})
	if err != nil {
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
	if res.Mutation == nil {
		res.Mutation = &gql.Mutation{Set: req.Mutation.Set, Del: req.Mutation.Del}
	}
	if res.Schema == nil {
		res.Schema = req.Schema
	}

	var queryRequest = query.QueryRequest{
		Latency:      &l,
		GqlQuery:     &res,
		SchemaUpdate: req.Mutation.Schema,
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
	if !*Config.Nomutations {
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
