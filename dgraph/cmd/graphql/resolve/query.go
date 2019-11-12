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
	"fmt"

	"github.com/golang/glog"
	otrace "go.opencensus.io/trace"

	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/api"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/x"
)

// A QueryResolver can resolve a single query.
type QueryResolver interface {
	Resolve(ctx context.Context, query schema.Query) *Resolved
}

// A QueryRewriter can build a Dgraph gql.GraphQuery from a GraphQL query,
type QueryRewriter interface {
	Rewrite(q schema.Query) (*gql.GraphQuery, error)
}

// A QueryExecutor can execute a gql.GraphQuery and return a result.  The result of
// a QueryExecutor doesn't need to be valid GraphQL results.
type QueryExecutor interface {
	Query(ctx context.Context, query *gql.GraphQuery) ([]byte, error)
}

// QueryResolverFunc is an adapter that allows to build a QueryResolver from
// a function.  Based on the http.HandlerFunc pattern.
type QueryResolverFunc func(ctx context.Context, query schema.Query) *Resolved

// QueryRewritingFunc is an adapter that allows us to build a QueryRewriter from
// a function.  Based on the http.HandlerFunc pattern.
type QueryRewritingFunc func(q schema.Query) (*gql.GraphQuery, error)

// QueryExecutionFunc is an adapter that allows us to compose query execution and
// build a QueryExecuter from a function.  Based on the http.HandlerFunc pattern.
type QueryExecutionFunc func(ctx context.Context, query *gql.GraphQuery) ([]byte, error)

// Resolve calls qr(ctx, query)
func (qr QueryResolverFunc) Resolve(ctx context.Context, query schema.Query) *Resolved {
	return qr(ctx, query)
}

// Rewrite calls qr(q)
func (qr QueryRewritingFunc) Rewrite(q schema.Query) (*gql.GraphQuery, error) {
	return qr(q)
}

// Query calls qe(ctx, query)
func (qe QueryExecutionFunc) Query(ctx context.Context, query *gql.GraphQuery) ([]byte, error) {
	return qe(ctx, query)
}

// NewQueryResolver creates a new query resolver.  The resolver runs the pipeline:
// 1) rewrite the query using qr (return error if failed)
// 2) execute the rewritten query with qe (return error if failed)
// 3) process the result with rc
func NewQueryResolver(qr QueryRewriter, qe QueryExecutor, rc ResultCompleter) QueryResolver {
	return &queryResolver{queryRewriter: qr, queryExecutor: qe, resultCompleter: rc}
}

// NoOpQueryExecution does nothing and returns nil.
func NoOpQueryExecution() QueryExecutionFunc {
	return QueryExecutionFunc(func(ctx context.Context, query *gql.GraphQuery) ([]byte, error) {
		return nil, nil
	})
}

// NoOpQueryRewrite does nothing and returns a nil rewriting.
func NoOpQueryRewrite() QueryRewritingFunc {
	return QueryRewritingFunc(func(q schema.Query) (*gql.GraphQuery, error) {
		return nil, nil
	})
}

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
