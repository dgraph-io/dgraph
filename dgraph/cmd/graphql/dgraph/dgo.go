/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
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

package dgraph

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/golang/glog"
	"github.com/vektah/gqlparser/gqlerror"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
)

// Client is the GraphQL API's view of the database.  Rather than relying on
// dgo, we rely on this abstraction and thus it's easier to run the GraphQL API
// in other environments: e.g. it could run on an alpha with no change to the
// GraphQL layer - would just need a implementation of this that forwards the
// GraphQuery straight in.  Similarly, we could allow the GraphQL API to push
// GraphQuery to alpha, so we don't have to stringify -> reparse etc.  Also
// allows exercising some of the particulars around GraphQL error processing
// without needing a Dgraph instance the reproduces the exact error conditions.
type Client interface {
	Query(ctx context.Context, query *QueryBuilder) ([]byte, error)
	Mutate(ctx context.Context, val interface{}) (map[string]string, error)
	DeleteNode(ctx context.Context, uid uint64) error
	AssertType(ctx context.Context, uid uint64, typ string) error
}

type dgraph struct {
	client *dgo.Dgraph
	// + transactions ???
}

// AsDgraph wraps a dgo client into the API's internal Dgraph representation.
func AsDgraph(dgo *dgo.Dgraph) Client {
	return &dgraph{client: dgo}
}

func (dg *dgraph) Query(ctx context.Context, query *QueryBuilder) ([]byte, error) {

	q, err := query.AsQueryString()
	if err != nil {
		return nil, schema.GQLWrapf(err, "couldn't build Dgraph query")
	}

	if glog.V(3) {
		glog.Infof("Executing Dgraph query: \n%s\n", q)
	}

	resp, err := dg.client.NewTxn().Query(ctx, q)
	return resp.Json, schema.GQLWrapf(err, "Dgraph query failed")
}

func (dg *dgraph) Mutate(ctx context.Context, val interface{}) (map[string]string, error) {

	jsonMu, err := json.Marshal(val)
	if err != nil {
		return nil, schema.GQLWrapf(err, "couldn't marshal mutation")
	}

	if glog.V(3) {
		glog.Infof("Executing Dgraph mutation: \n%s\n", jsonMu)
	}

	mu := &api.Mutation{
		CommitNow: true,
		SetJson:   jsonMu,
	}

	assigned, err := dg.client.NewTxn().Mutate(ctx, mu)
	return assigned.Uids, schema.GQLWrapf(err, "couldn't execute mutation")
}

// DeleteNode deletes a single node from the graph.
func (dg *dgraph) DeleteNode(ctx context.Context, uid uint64) error {
	// TODO: Note this simple cut that just removes it's outgoing edges.
	// If we are to do referential integrity or ensuring the type constraints
	// on the nodes, or cascading deletes, then a whole bunch more needs to be
	// done on deletion.
	//
	// This is safe though in the sense that once we have proper error propagation
	// we'd remove the node and if that caused any errors in the graph, that
	// would be picked up and handled as GraphQL errors in future queries, etc.

	mu := &api.Mutation{
		CommitNow: true,
		DelNquads: []byte(fmt.Sprintf("<0x%x> * * .", uid)),
	}

	_, err := dg.client.NewTxn().Mutate(ctx, mu)
	return err
}

// AssertType checks if uid is of type typ.  It returns nil if the type assertion
// holds, and an error if something goes wrong.
func (dg *dgraph) AssertType(ctx context.Context, uid uint64, typ string) error {

	qb := NewQueryBuilder().
		WithAttr("checkID").
		WithUIDRoot(uid).
		WithTypeFilter(typ).
		WithField("uid")

	resp, err := dg.Query(ctx, qb)
	if err != nil {
		return schema.GQLWrapf(err, "unable to type check id")
	}

	var decode struct {
		CheckID []struct {
			Uid string
		}
	}
	if err := json.Unmarshal(resp, &decode); err != nil {
		glog.Errorf("Failed to unmarshal typecheck query : %v+", err)
		return schema.GQLWrapf(err, "unable to type check id")
	}

	if len(decode.CheckID) != 1 {
		return gqlerror.Errorf("Node with id %s is not of type %s", string(uid), typ)
	}

	return nil
}
