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
	"time"

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
	timers          schema.TimerFactory
	query           schema.Query
}

func (qr *queryResolver) Resolve(ctx context.Context, query schema.Query) *Resolved {

	span := otrace.FromContext(ctx)
	stop := x.SpanTimer(span, "resolveQuery")
	defer stop()
	resolved:=&Resolved{}
	resolved.Extensions=&schema.Extensions{}
	resolved.Extensions.Tracing=&schema.Trace{}
	resolved.Extensions.Tracing.Execution = []*schema.ResolverTrace {
		{ParentType: "Query",
		FieldName:  query.ResponseName(),
		ReturnType: query.Type().String(),
		Path : []interface{}{query.ResponseName()},

	}}
	timers := schema.NewOffsetTimerFactory(ctx.Value("starttime").(time.Time))
	timer := timers.NewOffsetTimer(&resolved.Extensions.Tracing.Execution[0].OffsetDuration)
	timer.Start()
	defer timer.Stop()

	resolved1 := qr.rewriteAndExecute(ctx, query)
	if resolved1.Data == nil {
		resolved1.Data = map[string]interface{}{query.Name(): nil}
	}

	qr.resultCompleter.Complete(ctx, resolved)
	resolved.Extensions.Tracing.Execution[0].Dgraph=resolved1.Extensions.Tracing.Execution[0].Dgraph
	resolved1.Extensions.Tracing.Execution[0]=resolved.Extensions.Tracing.Execution[0]
	return resolved1
}

func (qr *queryResolver) rewriteAndExecute(ctx context.Context, query schema.Query) *Resolved {

	emptyResult := func(err error) *Resolved {
		return &Resolved{
			Data:  map[string]interface{}{query.Name(): nil},
			Field: query,
			Err:   err,
		}
	}

	dgQuery, err := qr.queryRewriter.Rewrite(ctx, query)
	if err != nil {
		return emptyResult(schema.GQLWrapf(err, "couldn't rewrite query %s",
			query.ResponseName()))
	}

	dgraphDuration := &schema.LabeledOffsetDuration{Label: "query"}
	startTime, _ := ctx.Value("starttime").(time.Time)
	timers1 := schema.NewOffsetTimerFactory(startTime)
	timer1 := timers1.NewOffsetTimer(&dgraphDuration.OffsetDuration)
	timer1.Start()

	resp, err := qr.executor.Execute(ctx,
		&dgoapi.Request{Query: dgraph.AsString(dgQuery), ReadOnly: true})

	timer1.Stop()

	if err != nil {
		glog.Infof("Dgraph query execution failed : %s", err)
		return emptyResult(schema.GQLWrapf(err, "Dgraph query failed"))
	}
	resolved:=&Resolved{}

	resolved = completeDgraphResult(ctx, query, resp.GetJson(), err)
	resolved.Extensions =
		&schema.Extensions{TouchedUids: resp.GetMetrics().GetNumUids()[touchedUidsKey]}
	resolved.Extensions.Tracing=&schema.Trace{}
	resolved.Extensions.Tracing.Execution = []*schema.ResolverTrace{{Dgraph:[]*schema.LabeledOffsetDuration{dgraphDuration}}}
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
