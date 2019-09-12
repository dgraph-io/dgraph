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
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/dgraph"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
)

// queryResolver can resolve a single GraphQL query field
type queryResolver struct {
	query         schema.Query
	schema        schema.Schema
	dgraph        dgraph.Client
	queryRewriter dgraph.QueryRewriter
	operation     schema.Operation
}

// resolve a query.
func (qr *queryResolver) resolve(ctx context.Context) *resolved {
	res := &resolved{}

	ctx, qspan := otrace.StartSpan(ctx, qr.query.Alias())
	defer func() {
		qspan.End()
	}()

	if qr.query.QueryType() == schema.SchemaQuery {
		resp, err := schema.Introspect(qr.operation, qr.query, qr.schema)
		if err != nil {
			res.err = err
			return res
		}
		// This is because Introspect returns an object.
		if len(resp) >= 2 {
			res.data = resp[1 : len(resp)-1]
		}
		return res
	}

	_, span := otrace.StartSpan(ctx, "queryRewriter")
	dgQuery, err := qr.queryRewriter.Rewrite(qr.query)
	if err != nil {
		res.err = schema.GQLWrapf(err, "couldn't rewrite query")
		span.End()
		return res
	}
	span.End()

	spanCtx, span := otrace.StartSpan(ctx, "dgraph.Query")
	resp, err := qr.dgraph.Query(spanCtx, dgQuery)
	if err != nil {
		glog.Infof("[%s] Dgraph query failed : %s", api.RequestID(ctx), err)
		res.err = schema.GQLWrapf(err, "[%s] failed to resolve query", api.RequestID(ctx))
		span.End()
		return res
	}
	span.End()

	spanCtx, span = otrace.StartSpan(ctx, "completeDgraphResult")
	completed, err := completeDgraphResult(spanCtx, qr.query, resp)
	span.End()
	res.err = err

	// chop leading '{' and trailing '}' from JSON object
	//
	// The final GraphQL result gets built like
	// { data:
	//    {
	//      q1: {...},
	//      q2: [ {...}, {...} ],
	//      ...
	//    }
	// }
	// Here we are building a single one of the q's, so the fully resolved
	// result should be q1: {...}, rather than {q1: {...}}.
	//
	// completeDgraphResult() always returns a valid JSON object.  At least
	// { q: null }
	// even in error cases, so this is safe.
	res.data = completed[1 : len(completed)-1]
	return res
}
