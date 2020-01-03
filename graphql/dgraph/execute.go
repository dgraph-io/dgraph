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
	"strings"

	"github.com/golang/glog"
	"go.opencensus.io/trace"

	dgoapi "github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/graphql/api"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/x"
)

// Query is the underlying dgraph implementation of QueryExecutor.
func Query(ctx context.Context, query *gql.GraphQuery) ([]byte, error) {
	span := trace.FromContext(ctx)
	stop := x.SpanTimer(span, "dgraph.Query")
	defer stop()

	queryStr := AsString(query)

	if glog.V(3) {
		glog.Infof("[%s] Executing Dgraph query: \n%s\n", api.RequestID(ctx), queryStr)
	}

	req := &dgoapi.Request{
		Query: queryStr,
	}
	resp, err := (&edgraph.Server{}).Query(ctx, req)
	return resp.GetJson(), schema.GQLWrapf(err, "Dgraph query failed")
}

// Mutate is the underlying dgraph implementation of MutationExecutor.
func Mutate(
	ctx context.Context,
	query *gql.GraphQuery,
	mutations []*dgoapi.Mutation) (map[string]string, map[string]interface{}, error) {

	span := trace.FromContext(ctx)
	stop := x.SpanTimer(span, "dgraph.Mutate")
	defer stop()

	if query == nil && len(mutations) == 0 {
		return nil, nil, nil
	}

	queryStr := AsString(query)

	if glog.V(3) {
		muts := make([]string, len(mutations))
		for i, m := range mutations {
			muts[i] = m.String()
		}

		glog.Infof("[%s] Executing Dgraph mutation; with\nQuery: \n%s\nMutations:%s",
			api.RequestID(ctx), queryStr, strings.Join(muts, "\n"))
	}

	req := &dgoapi.Request{
		Query:     queryStr,
		CommitNow: true,
		Mutations: mutations,
	}
	resp, err := (&edgraph.Server{}).Query(ctx, req)
	if err != nil {
		return nil, nil, schema.GQLWrapf(err, "Dgraph mutation failed")
	}

	result := make(map[string]interface{})
	if query != nil && len(resp.GetJson()) != 0 {
		if err := json.Unmarshal(resp.GetJson(), &result); err != nil {
			return nil, nil,
				schema.GQLWrapf(err, "Couldn't unmarshal response from Dgraph mutation")
		}
	}
	return resp.GetUids(), result, schema.GQLWrapf(err, "Dgraph mutation failed")
}
