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

// a queryResolver can resolve a single GraphQL query field.
type queryResolver struct {
	queryRewriter   QueryRewriter
	queryExecutor   QueryExecutor
	resultCompleter ResultCompleter
}

func (qr *queryResolver) Resolve(ctx context.Context, query schema.Query) (*Resolved, bool) {

	span := otrace.FromContext(ctx)
	stop := x.SpanTimer(span, "resolveQuery")
	defer stop()

	resCtx := &ResolverContext{Ctx: ctx, RootField: query}

	res, succeed, err := qr.rewriteAndExecute(resCtx, query)

	fmt.Println(string(res))

	completed, err := qr.resultCompleter.Complete(resCtx, query, res, err)
	return &Resolved{
		Data: completed,
		Err:  err,
	}, succeed
}

func (qr *queryResolver) rewriteAndExecute(
	resCtx *ResolverContext, query schema.Query) ([]byte, bool, error) {

	dgQuery, err := qr.queryRewriter.Rewrite(query)
	if err != nil {
		return nil, resolverFailed,
			schema.GQLWrapf(err, "couldn't rewrite query %s", query.ResponseName())
	}

	resp, err := qr.queryExecutor.Query(resCtx, dgQuery)
	if err != nil {
		glog.Infof("[%s] query execution failed : %s", api.RequestID(resCtx.Ctx), err)
		return nil, resolverFailed,
			schema.GQLWrapf(err, "[%s] failed to resolve query", api.RequestID(resCtx.Ctx))
	}

	return resp, resolverSucceeded, nil
}

func introspectionExecution(resCtx *ResolverContext, query *gql.GraphQuery) ([]byte, error) {
	return schema.Introspect(resCtx.RootField)
}
