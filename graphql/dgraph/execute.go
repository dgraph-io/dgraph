/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package dgraph

import (
	"context"
	"strings"

	"github.com/golang/glog"
	"go.opencensus.io/trace"

	dgoapi "github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/hypermodeinc/dgraph/v25/edgraph"
	"github.com/hypermodeinc/dgraph/v25/graphql/schema"
	"github.com/hypermodeinc/dgraph/v25/x"
)

type DgraphEx struct{}

// Execute is the underlying dgraph implementation of Dgraph execution.
// If field is nil, returned response has JSON in DQL form, otherwise it will be in GraphQL form.
func (dg *DgraphEx) Execute(ctx context.Context, req *dgoapi.Request,
	field schema.Field) (*dgoapi.Response, error) {

	span := trace.FromContext(ctx)
	stop := x.SpanTimer(span, "dgraph.Execute")
	defer stop()

	if req == nil || (req.Query == "" && len(req.Mutations) == 0) {
		return nil, nil
	}

	if glog.V(3) {
		muts := make([]string, len(req.Mutations))
		for i, m := range req.Mutations {
			muts[i] = m.String()
		}

		glog.Infof("Executing Dgraph request; with\nQuery: \n%s\nMutations:%s",
			req.Query, strings.Join(muts, "\n"))
	}

	ctx = context.WithValue(ctx, edgraph.IsGraphql, true)
	resp, err := (&edgraph.Server{}).QueryGraphQL(ctx, req, field)
	if !x.IsGqlErrorList(err) {
		err = schema.GQLWrapf(err, "Dgraph execution failed")
	}

	return resp, err
}

// CommitOrAbort is the underlying dgraph implementation for committing a Dgraph transaction
func (dg *DgraphEx) CommitOrAbort(ctx context.Context,
	tc *dgoapi.TxnContext) (*dgoapi.TxnContext, error) {
	return (&edgraph.Server{}).CommitOrAbort(ctx, tc)
}
