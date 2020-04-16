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
	"encoding/json"

	"github.com/golang/glog"
	otrace "go.opencensus.io/trace"

	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/x"
)

// A QueryResolver can resolve a single query.
type QueryResolver interface {
	Resolve(ctx context.Context, query schema.Query) *Resolved
}

// A QueryRewriter can build a Dgraph gql.GraphQuery from a GraphQL query,
type QueryRewriter interface {
	Rewrite(ctx context.Context, q schema.Query) (*gql.GraphQuery, error)
}

// A QueryExecutor can execute a gql.GraphQuery and return a result.  The result of
// a QueryExecutor doesn't need to be valid GraphQL results.
type QueryExecutor interface {
	Query(ctx context.Context, query *gql.GraphQuery) ([]byte, *schema.Extensions, error)
}

// QueryResolverFunc is an adapter that allows to build a QueryResolver from
// a function.  Based on the http.HandlerFunc pattern.
type QueryResolverFunc func(ctx context.Context, query schema.Query) *Resolved

// QueryRewritingFunc is an adapter that allows us to build a QueryRewriter from
// a function.  Based on the http.HandlerFunc pattern.
type QueryRewritingFunc func(ctx context.Context, q schema.Query) (*gql.GraphQuery, error)

// QueryExecutionFunc is an adapter that allows us to compose query execution and
// build a QueryExecuter from a function.  Based on the http.HandlerFunc pattern.
type QueryExecutionFunc func(ctx context.Context, query *gql.GraphQuery) ([]byte,
	*schema.Extensions, error)

// Resolve calls qr(ctx, query)
func (qr QueryResolverFunc) Resolve(ctx context.Context, query schema.Query) *Resolved {
	return qr(ctx, query)
}

// Rewrite calls qr(q)
func (qr QueryRewritingFunc) Rewrite(ctx context.Context, q schema.Query) (*gql.GraphQuery, error) {
	return qr(ctx, q)
}

// Query calls qe(ctx, query)
func (qe QueryExecutionFunc) Query(ctx context.Context, query *gql.GraphQuery) ([]byte,
	*schema.Extensions, error) {
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
	return QueryExecutionFunc(func(ctx context.Context, query *gql.GraphQuery) ([]byte,
		*schema.Extensions, error) {
		return nil, nil, nil
	})
}

// NoOpQueryRewrite does nothing and returns a nil rewriting.
func NoOpQueryRewrite() QueryRewritingFunc {
	return QueryRewritingFunc(func(ctx context.Context, q schema.Query) (*gql.GraphQuery, error) {
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

	resolved := qr.rewriteAndExecute(ctx, query)
	if resolved.Data == nil {
		resolved.Data = map[string]interface{}{query.ResponseName(): nil}
	}

	qr.resultCompleter.Complete(ctx, resolved)
	return resolved
}

func (qr *queryResolver) rewriteAndExecute(ctx context.Context, query schema.Query) *Resolved {

	emptyResult := func(err error) *Resolved {
		return &Resolved{
			Data:  map[string]interface{}{query.ResponseName(): nil},
			Field: query,
			Err:   err,
		}
	}

	dgQuery, err := qr.queryRewriter.Rewrite(ctx, query)
	if err != nil {
		return emptyResult(schema.GQLWrapf(err, "couldn't rewrite query %s",
			query.ResponseName()))
	}

	resp, ext, err := qr.queryExecutor.Query(ctx, dgQuery)
	if err != nil {
		glog.Infof("Dgraph query execution failed : %s", err)
		return emptyResult(schema.GQLWrapf(err, "Dgraph query failed"))
	}

	// FIXME: just to get it running for now - this should have it's own .Resolve()
	if query.QueryType() == schema.SchemaQuery {
		var result map[string]interface{}
		var err2 error
		if len(resp) > 0 {
			err2 = json.Unmarshal(resp, &result)
		}

		return &Resolved{
			Data:       result,
			Field:      query,
			Err:        schema.AppendGQLErrs(err, err2),
			Extensions: ext,
		}
	}

	resolved := completeDgraphResult(ctx, query, resp, err)
	resolved.Extensions = ext

	return resolved
}

func introspectionExecution(q schema.Query) QueryExecutionFunc {
	return QueryExecutionFunc(func(ctx context.Context, query *gql.GraphQuery) ([]byte,
		*schema.Extensions, error) {
		data, err := schema.Introspect(q)
		return data, nil, err
	})
}
