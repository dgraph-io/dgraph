/*
 * Copyright 2016 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package worker

import (
	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/protos/graphp"
	"github.com/dgraph-io/dgraph/protos/taskp"
	"github.com/dgraph-io/dgraph/protos/workerp"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

var (
	emptySchemaResult taskp.SchemaResult
)

type resultErr struct {
	result *taskp.SchemaResult
	err    error
}

// getSchema iterates over all predicates and populates the asked fields, if list of
// predicates is not specified, then all the predicates belonging to the group
// are returned
func getSchema(ctx context.Context, s *taskp.Schema) (*taskp.SchemaResult, error) {
	var result taskp.SchemaResult
	var predicates []string
	if len(s.Predicates) > 0 {
		predicates = s.Predicates
	} else {
		predicates = schema.State().Predicates(s.GroupId)
	}

	for _, attr := range predicates {
		if !groups().ServesGroup(group.BelongsTo(attr)) {
			return &emptySchemaResult,
				x.Errorf("Predicate fingerprint doesn't match this instance")
		}
		if schemaNode := populateSchema(attr, s.Fields); schemaNode != nil {
			result.Schema = append(result.Schema, schemaNode)
		}

	}
	return &result, nil
}

// populateSchema returns the information of asked fields for given attribute
func populateSchema(attr string, fields []string) *graphp.SchemaNode {
	var schemaNode graphp.SchemaNode
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
		default:
			//pass
		}
	}
	return &schemaNode
}

// addToSchemaMap groups the predicates by group id, if list of predicates is
// empty then it adds all known groups
func addToSchemaMap(schemaMap map[uint32]*taskp.Schema, schema *graphp.Schema) {
	for _, attr := range schema.Predicates {
		gid := group.BelongsTo(attr)
		s := schemaMap[gid]
		if s == nil {
			s = &taskp.Schema{GroupId: gid}
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
			s = &taskp.Schema{GroupId: gid}
			s.Fields = schema.Fields
			schemaMap[gid] = s
		}
	}
}

// If the current node serves the group serve the schema or forward
// to relevant node
// TODO: Janardhan - if read fails try other servers serving same group
func getSchemaOverNetwork(ctx context.Context, gid uint32, s *taskp.Schema,
	ch chan *resultErr) {
	if groups().ServesGroup(gid) {
		schema, e := getSchema(ctx, s)
		ch <- &resultErr{result: schema, err: e}
		return
	}

	_, addr := groups().Leader(gid)
	pl := pools().get(addr)
	conn, e := pl.Get()
	if e != nil {
		ch <- &resultErr{err: e}
		return
	}
	defer pl.Put(conn)

	c := workerp.NewWorkerClient(conn)
	schema, e := c.Schema(ctx, s)
	ch <- &resultErr{result: schema, err: e}
}

// GetSchemaOverNetwork checks which group should be serving the schema
// according to fingerprint of the predicate and sends it to that instance.
func GetSchemaOverNetwork(ctx context.Context, schema *graphp.Schema) ([]*graphp.SchemaNode, error) {
	schemaMap := make(map[uint32]*taskp.Schema)
	addToSchemaMap(schemaMap, schema)

	results := make(chan *resultErr, len(schemaMap))
	var schemaNodes []*graphp.SchemaNode

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
func (w *grpcWorker) Schema(ctx context.Context, s *taskp.Schema) (*taskp.SchemaResult, error) {
	if ctx.Err() != nil {
		return &emptySchemaResult, ctx.Err()
	}

	if !groups().ServesGroup(s.GroupId) {
		return &emptySchemaResult, x.Errorf("This server doesn't serve group id: %v", s.GroupId)
	}
	return getSchema(ctx, s)
}
