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

package resolve

import (
	"context"

	"github.com/golang/glog"
	otrace "go.opencensus.io/trace"

	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/api"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/x"
)

// a queryResolver can resolve a single GraphQL query field.
type queryResolver struct {
	query           schema.Query
	queryRewriter   QueryRewriter
	queryExecutor   QueryExecutor
	resultCompleter ResultCompleter
}

// a schemaIntrospector is the QueryExecutor function of schema introspection.
type schemaIntrospector struct {
	query     schema.Query
	schema    schema.Schema
	operation schema.Operation
}

func (qr *queryResolver) Resolve(ctx context.Context) (*Resolved, bool) {

	span := otrace.FromContext(ctx)
	stop := x.SpanTimer(span, "resolveQuery")
	defer stop()

	dgQuery, err := qr.queryRewriter.Rewrite(qr.query)
	if err != nil {
		return &Resolved{
			Err: schema.GQLWrapf(err, "couldn't rewrite query %s", qr.query.ResponseName()),
		}, resolverFailed
	}

	resp, err := qr.queryExecutor.Query(ctx, dgQuery)
	if err != nil {
		glog.Infof("[%s] Dgraph query failed : %s", api.RequestID(ctx), err)
		return &Resolved{
			Err: schema.GQLWrapf(err, "[%s] failed to resolve query", api.RequestID(ctx)),
		}, resolverFailed
	}

	completed, err := qr.resultCompleter.Complete(ctx, qr.query, resp)
	return &Resolved{
		Data: completed,
		Err:  err,
	}, resolverSucceeded
}

func (si *schemaIntrospector) Query(ctx context.Context, query *gql.GraphQuery) ([]byte, error) {
	return schema.Introspect(si.operation, si.query, si.schema)
}
