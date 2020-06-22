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

	dgoapi "github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/graphql/dgraph"
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

// QueryResolverFunc is an adapter that allows to build a QueryResolver from
// a function.  Based on the http.HandlerFunc pattern.
type QueryResolverFunc func(ctx context.Context, query schema.Query) *Resolved

// Resolve calls qr(ctx, query)
func (qr QueryResolverFunc) Resolve(ctx context.Context, query schema.Query) *Resolved {
	return qr(ctx, query)
}

// NewQueryResolver creates a new query resolver.  The resolver runs the pipeline:
// 1) rewrite the query using qr (return error if failed)
// 2) execute the rewritten query with qe (return error if failed)
// 3) process the result with rc
func NewQueryResolver(qr QueryRewriter, ex DgraphExecutor, rc ResultCompleter) QueryResolver {
	return &queryResolver{queryRewriter: qr, executor: ex, resultCompleter: rc}
}

// a queryResolver can resolve a single GraphQL query field.
type queryResolver struct {
	queryRewriter   QueryRewriter
	executor        DgraphExecutor
	resultCompleter ResultCompleter
}

func (qr *queryResolver) Resolve(ctx context.Context, query schema.Query) *Resolved {
	span := otrace.FromContext(ctx)
	stop := x.SpanTimer(span, "resolveQuery")
	defer stop()

	resolverTrace := &schema.ResolverTrace{
		Path:       []interface{}{query.ResponseName()},
		ParentType: "Query",
		FieldName:  query.ResponseName(),
		ReturnType: query.Type().String(),
	}
	timer := newtimer(ctx, &resolverTrace.OffsetDuration)
	timer.Start()
	defer timer.Stop()

	resolved := qr.rewriteAndExecute(ctx, query)
	if resolved.Data == nil {
		resolved.Data = map[string]interface{}{query.Name(): nil}
	}

	qr.resultCompleter.Complete(ctx, resolved)
	resolverTrace.Dgraph = resolved.Extensions.Tracing.Execution.Resolvers[0].Dgraph
	resolved.Extensions.Tracing.Execution.Resolvers[0] = resolverTrace
	return resolved
}

func (qr *queryResolver) rewriteAndExecute(ctx context.Context, query schema.Query) *Resolved {
	dgraphQueryDuration := &schema.LabeledOffsetDuration{Label: "query"}
	ext := &schema.Extensions{
		Tracing: &schema.Trace{
			Execution: &schema.ExecutionTrace{
				Resolvers: []*schema.ResolverTrace{
					{Dgraph: []*schema.LabeledOffsetDuration{dgraphQueryDuration}},
				},
			},
		},
	}

	emptyResult := func(err error) *Resolved {
		return &Resolved{
			Data:       map[string]interface{}{query.Name(): nil},
			Field:      query,
			Err:        err,
			Extensions: ext,
		}
	}

	dgQuery, err := qr.queryRewriter.Rewrite(ctx, query)
	if err != nil {
		return emptyResult(schema.GQLWrapf(err, "couldn't rewrite query %s",
			query.ResponseName()))
	}

	queryTimer := newtimer(ctx, &dgraphQueryDuration.OffsetDuration)
	queryTimer.Start()
	resp, err := qr.executor.Execute(ctx, &dgoapi.Request{Query: dgraph.AsString(dgQuery),
		ReadOnly: true})
	queryTimer.Stop()

	if err != nil {
		glog.Infof("Dgraph query execution failed : %s", err)
		return emptyResult(schema.GQLWrapf(err, "Dgraph query failed"))
	}

	ext.TouchedUids = resp.GetMetrics().GetNumUids()[touchedUidsKey]
	resolved := completeDgraphResult(ctx, query, resp.GetJson(), err)
	resolved.Extensions = ext

	return resolved
}

func resolveIntrospection(ctx context.Context, q schema.Query) *Resolved {
	data, err := schema.Introspect(q)

	var result map[string]interface{}
	var err2 error
	if len(data) > 0 {
		err2 = json.Unmarshal(data, &result)
	}

	return &Resolved{
		Data:  result,
		Field: q,
		Err:   schema.AppendGQLErrs(err, err2),
	}
}
