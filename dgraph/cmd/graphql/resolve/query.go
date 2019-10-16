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
	queryRewriter   QueryRewriter
	queryExecutor   QueryExecutor
	resultCompleter ResultCompleter
}

func (qr *queryResolver) Resolve(ctx context.Context, query schema.Query) *Resolved {
	span := otrace.FromContext(ctx)
	stop := x.SpanTimer(span, "resolveQuery")
	defer stop()

	res, err := qr.rewriteAndExecute(ctx, query)

	completed, err := qr.resultCompleter.Complete(ctx, query, res, err)
	return &Resolved{Data: completed, Err: err}
}

func (qr *queryResolver) rewriteAndExecute(
	ctx context.Context, query schema.Query) ([]byte, error) {

	dgQuery, err := qr.queryRewriter.Rewrite(query)
	if err != nil {
		return nil, schema.GQLWrapf(err, "couldn't rewrite query %s", query.ResponseName())
	}

	resp, err := qr.queryExecutor.Query(ctx, dgQuery)
	if err != nil {
		glog.Infof("[%s] query execution failed : %s", api.RequestID(ctx), err)
		return nil, schema.GQLWrapf(err, "[%s] failed to resolve query", api.RequestID(ctx))
	}

	return resp, nil
}

func introspectionExecution(q schema.Query) QueryExecutionFunc {
	return QueryExecutionFunc(func(ctx context.Context, query *gql.GraphQuery) ([]byte, error) {
		return schema.Introspect(q)
	})
}
