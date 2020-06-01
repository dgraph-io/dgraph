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
	//resolved := &Resolved{}
	//Add Tracing of the complete query
	// trace, timer := traceWithTimer(qr.timers, qr.query, "Query") //getting panic

	resolved := qr.rewriteAndExecute(ctx, query)
	if resolved.Data == nil {
		resolved.Data = map[string]interface{}{query.Name(): nil}
	}

	// resolved.trace. = []*schema.ResolverTrace{
	// 	ParentType: "Query",
	// 	FieldName:  "getmessage",
	// 	ReturnType: "message",                                       //dummy data to test
	// }
	// resolved.trace.offsetDuration.StartOffset = 1000
	// resolved.trace.offsetDuration.Duration = 150
	// resolved.trace.Dgraph.Label = "Dgraph"
	// resolved.trace.Dgraph.offsetDuration.StartOffset = 500
	// resolved.trace.Dgraph.offsetDuration.Duration = 100

	qr.resultCompleter.Complete(ctx, resolved)

	return resolved
}

func (qr *queryResolver) rewriteAndExecute(ctx context.Context, query schema.Query) *Resolved {

	emptyResult := func(err error) *Resolved {
		return &Resolved{
			Data:  map[string]interface{}{query.Name(): nil},
			Field: query,
			Err:   err,
		}
	}

	trace := &schema.ResolverTrace{
		ParentType: "Query",
		FieldName:  query.ResponseName(),
		ReturnType: query.Type().String(),
	}
	trace.Path = []interface{}{query.ResponseName()}
	timers := schema.NewOffsetTimerFactory(time.Now())
	timer := timers.NewOffsetTimer(&trace.OffsetDuration)
	timer.Start()
	defer timer.Stop()

	dgQuery, err := qr.queryRewriter.Rewrite(ctx, query)
	if err != nil {
		return emptyResult(schema.GQLWrapf(err, "couldn't rewrite query %s",
			query.ResponseName()))
	}

	//Add Trace for the execution path of the query

	dgraphDuration := &schema.LabeledOffsetDuration{Label: "query"}
	timers1 := schema.NewOffsetTimerFactory(time.Now())
	timer1 := timers1.NewOffsetTimer(&dgraphDuration.OffsetDuration)
	// var tf1 schema.TimerFactory
	//timer1 := tf1.NewOffsetTimer(&dgraphDuration.OffsetDuration)
	timer1.Start()

	resp, err := qr.executor.Execute(ctx,
		&dgoapi.Request{Query: dgraph.AsString(dgQuery), ReadOnly: true})

	timer1.Stop()
	trace.Dgraph = []*schema.LabeledOffsetDuration{dgraphDuration}
	if err != nil {
		glog.Infof("Dgraph query execution failed : %s", err)
		return emptyResult(schema.GQLWrapf(err, "Dgraph query failed"))
	}

	resolved := completeDgraphResult(ctx, query, resp.GetJson(), err)
	resolved.trace = []*schema.ResolverTrace{trace}

	// resolved.Extensions =
	// 	&schema.Extensions{TouchedUids: resp.GetMetrics().GetNumUids()[touchedUidsKey]}

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
