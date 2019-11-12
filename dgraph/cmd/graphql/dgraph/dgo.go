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

	"github.com/golang/glog"
	"go.opencensus.io/trace"

	"github.com/dgraph-io/dgo/v2"
	dgoapi "github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/api"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/x"
)

// Query is the underlying dgo implementation of QueryExecutor.
func Query(ctx context.Context, client *dgo.Dgraph, query *gql.GraphQuery) ([]byte, error) {
	span := trace.FromContext(ctx)
	stop := x.SpanTimer(span, "dgraph.Query")
	defer stop()

	queryStr := AsString(query)

	if glog.V(3) {
		glog.Infof("[%s] Executing Dgraph query: \n%s\n", api.RequestID(ctx), queryStr)
	}

	resp, err := client.NewTxn().
		Query(context.Background(), queryStr)

	return resp.GetJson(), schema.GQLWrapf(err, "Dgraph query failed")
}

// Mutate is the underlying dgo implementation of MutationExecutor.
func Mutate(
	ctx context.Context,
	client *dgo.Dgraph,
	query *gql.GraphQuery,
	mutations []*dgoapi.Mutation) (map[string]string, map[string][]string, error) {

	span := trace.FromContext(ctx)
	stop := x.SpanTimer(span, "dgraph.Mutate")
	defer stop()

	queryStr := AsString(query)

	if glog.V(3) {
		b, err := json.Marshal(mutations)
		if err != nil {
			glog.Infof("[%s] Failed to marshal mutations for logging: %s", api.RequestID(ctx), err)
			b = []byte("unable to marshal mutations for logging")
		}
		glog.Infof("[%s] Executing Dgraph mutation; with Query: \n%s\nwith mutations: \n%s\n",
			api.RequestID(ctx), queryStr, string(b))
	}

	req := &dgoapi.Request{
		Query:     queryStr,
		CommitNow: true,
		Mutations: mutations,
	}
	resp, err := client.NewTxn().Do(ctx, req)

	vars := make(map[string][]string, len(resp.GetVars()))
	for k, v := range resp.GetVars() {
		vars[k] = v.GetUids()
	}
	return resp.GetUids(), vars, schema.GQLWrapf(err, "Dgraph mutation failed")
}
