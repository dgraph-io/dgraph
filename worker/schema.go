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

package worker

import (
	"golang.org/x/net/context"
	"golang.org/x/net/trace"

	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

var (
	emptySchemaResult protos.SchemaResult
)

type resultErr struct {
	result *protos.SchemaResult
	err    error
}

// getSchema iterates over all predicates and populates the asked fields, if list of
// predicates is not specified, then all the predicates belonging to the group
// are returned
func getSchema(ctx context.Context, s *protos.SchemaRequest) (*protos.SchemaResult, error) {
	var result protos.SchemaResult
	var predicates []string
	var fields []string
	if len(s.Predicates) > 0 {
		predicates = s.Predicates
	} else {
		predicates = schema.State().Predicates(s.GroupId)
	}
	if len(s.Fields) > 0 {
		fields = s.Fields
	} else {
		fields = []string{"type", "index", "tokenizer", "reverse", "count", "list"}
	}

	for _, attr := range predicates {
		if !groups().ServesGroup(group.BelongsTo(attr)) {
			return &emptySchemaResult,
				x.Errorf("Predicate fingerprint doesn't match this instance")
		}
		if schemaNode := populateSchema(attr, fields); schemaNode != nil {
			result.Schema = append(result.Schema, schemaNode)
		}
	}
	return &result, nil
}

// populateSchema returns the information of asked fields for given attribute
func populateSchema(attr string, fields []string) *protos.SchemaNode {
	var schemaNode protos.SchemaNode
	var typ types.TypeID
	var err error
	if typ, err = schema.State().TypeOf(attr); err != nil {
		// schema is not defined
		return nil
	}
	schemaNode.Predicate = attr
	for _, field := range fields {
		switch field {
		case "type":
			schemaNode.Type = typ.Name()
		case "index":
			schemaNode.Index = schema.State().IsIndexed(attr)
		case "tokenizer":
			if schema.State().IsIndexed(attr) {
				schemaNode.Tokenizer = schema.State().TokenizerNames(attr)
			}
		case "reverse":
			schemaNode.Reverse = schema.State().IsReversed(attr)
		case "count":
			schemaNode.Count = schema.State().HasCount(attr)
		case "list":
			schemaNode.List = schema.State().IsList(attr)
		default:
			//pass
		}
	}
	return &schemaNode
}

// addToSchemaMap groups the predicates by group id, if list of predicates is
// empty then it adds all known groups
func addToSchemaMap(schemaMap map[uint32]*protos.SchemaRequest, schema *protos.SchemaRequest) {
	for _, attr := range schema.Predicates {
		gid := group.BelongsTo(attr)
		s := schemaMap[gid]
		if s == nil {
			s = &protos.SchemaRequest{GroupId: gid}
			s.Fields = schema.Fields
			schemaMap[gid] = s
		}
		s.Predicates = append(s.Predicates, attr)
	}
	if len(schema.Predicates) > 0 {
		return
	}
	// TODO: Janardhan - node shouldn't serve any request until membership
	// information is synced, should we fail health check till then ?
	gids := groups().KnownGroups()
	for _, gid := range gids {
		if gid == 0 {
			continue
		}
		s := schemaMap[gid]
		if s == nil {
			s = &protos.SchemaRequest{GroupId: gid}
			s.Fields = schema.Fields
			schemaMap[gid] = s
		}
	}
}

// If the current node serves the group serve the schema or forward
// to relevant node
// TODO: Janardhan - if read fails try other servers serving same group
func getSchemaOverNetwork(ctx context.Context, gid uint32, s *protos.SchemaRequest, ch chan resultErr) {
	if groups().ServesGroup(gid) {
		schema, e := getSchema(ctx, s)
		ch <- resultErr{result: schema, err: e}
		return
	}

	_, addr := groups().Leader(gid)
	pl, err := conn.Get().Get(addr)
	if err != nil {
		ch <- resultErr{err: err}
		return
	}
	defer conn.Get().Release(pl)
	conn := pl.Get()
	c := protos.NewWorkerClient(conn)
	schema, e := c.Schema(ctx, s)
	ch <- resultErr{result: schema, err: e}
}

// GetSchemaOverNetwork checks which group should be serving the schema
// according to fingerprint of the predicate and sends it to that instance.
func GetSchemaOverNetwork(ctx context.Context, schema *protos.SchemaRequest) ([]*protos.SchemaNode, error) {
	if err := x.HealthCheck(); err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Request rejected %v", err)
		}
		return nil, err
	}
	schemaMap := make(map[uint32]*protos.SchemaRequest)
	addToSchemaMap(schemaMap, schema)

	results := make(chan resultErr, len(schemaMap))
	var schemaNodes []*protos.SchemaNode

	for gid, s := range schemaMap {
		go getSchemaOverNetwork(ctx, gid, s, results)
	}

	// wait for all the goroutines to reply back.
	// we return if an error was returned or the parent called ctx.Done()
	for i := 0; i < len(schemaMap); i++ {
		select {
		case r := <-results:
			if r.err != nil {
				return nil, r.err
			}
			schemaNodes = append(schemaNodes, r.result.Schema...)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	close(results)

	return schemaNodes, nil
}

// Schema is used to get schema information over the network on other instances.
func (w *grpcWorker) Schema(ctx context.Context, s *protos.SchemaRequest) (*protos.SchemaResult, error) {
	if ctx.Err() != nil {
		return &emptySchemaResult, ctx.Err()
	}

	if !groups().ServesGroup(s.GroupId) {
		return &emptySchemaResult, x.Errorf("This server doesn't serve group id: %v", s.GroupId)
	}
	return getSchema(ctx, s)
}
