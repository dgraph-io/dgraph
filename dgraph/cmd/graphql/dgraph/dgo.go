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

package dgraph

import (
	"context"

	"github.com/golang/glog"
	"go.opencensus.io/trace"
	"google.golang.org/grpc/metadata"

	"github.com/dgraph-io/dgo"
	dgoapi "github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/api"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/x"
)

// Client is the GraphQL API's view of the database.  Rather than relying on
// dgo, we rely on this abstraction and thus it's easier to run the GraphQL API
// in other environments: e.g. it could run on an alpha with no change to the
// GraphQL layer - would just need a implementation of this that forwards the
// GraphQuery straight in.  Similarly, we could allow the GraphQL API to push
// GraphQuery to alpha, so we don't have to stringify -> reparse etc.  Also
// allows exercising some of the particulars around GraphQL error processing
// without needing a Dgraph instance the reproduces the exact error conditions.
//
// A Dgraph client is an implementation of both the resolve.QueryExecutor and the
// resolve.MutationExecutor interfaces.
type Client interface {
	Query(ctx context.Context, query *gql.GraphQuery) ([]byte, error)
	Mutate(ctx context.Context,
		query *gql.GraphQuery,
		mutations []*dgoapi.Mutation) (map[string]string, map[string][]string, error)
}

type dgraph struct {
	client *dgo.Dgraph
	// + transactions ???
}

// AsDgraph wraps a dgo client into the API's internal Dgraph representation.
func AsDgraph(dgo *dgo.Dgraph) Client {
	return &dgraph{client: dgo}
}

func (dg *dgraph) Query(ctx context.Context, query *gql.GraphQuery) ([]byte, error) {
	span := trace.FromContext(ctx)
	stop := x.SpanTimer(span, "dgraph.Query")
	defer stop()

	queryStr := asString(query)

	if glog.V(3) {
		glog.Infof("[%s] Executing Dgraph query: \n%s\n", api.RequestID(ctx), queryStr)
	}

	// Always use debug mode so that UID is inserted for every node that we touch
	// Otherwise we can't tell the difference in a query result between a node that's
	// missing and a node that's missing a single value.  E.g. if we are asking
	// for an Author and only the 'text' of all their posts
	// e.g. getAuthor(id: 0x123) { posts { text } }
	// If the author has 10 posts but three of them have a title, but no text,
	// then Dgraph would just return 7 posts.  And we'd have no way of knowing if
	// there's only 7 posts, or if there's more that are missing 'text'.
	// But, for GraphQL, we want to know about those missing values.
	md := metadata.Pairs("debug", "true")
	resp, err := dg.client.NewTxn().
		Query(metadata.NewOutgoingContext(ctx, md), queryStr)

	return resp.GetJson(), schema.GQLWrapf(err, "Dgraph query failed")
}

func (dg *dgraph) Mutate(
	ctx context.Context,
	query *gql.GraphQuery,
	mutations []*dgoapi.Mutation) (map[string]string, map[string][]string, error) {

	span := trace.FromContext(ctx)
	stop := x.SpanTimer(span, "dgraph.Mutate")
	defer stop()

	// if it's got just a query, then it's an error ???
	// ...this indicates a bug...

	if glog.V(3) {
		// FIXME: ...write out the whole thing

		// glog.Infof("[%s] Executing Dgraph upsert : \n%s\nwith set - %s\n",
		// 	api.RequestID(ctx), query, string(b))
	}

	req := &dgoapi.Request{
		Query:     asString(query),
		CommitNow: true,
		Mutations: mutations,
	}
	resp, err := dg.client.NewTxn().Do(ctx, req)
	return resp.GetUids(), nil, schema.GQLWrapf(err, "Dgraph mutation failed")
}
