/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */
package edgraph

import (
	"github.com/dgraph-io/dgraph/protos/api"
	"golang.org/x/net/context"

	"google.golang.org/grpc"
)

// inmemoryClient implements api.DgraphClient (it's equivalent of default grpc client, but for
// in-memory communication (without data (un)marshaling)).
type inmemoryClient struct {
	srv *Server
}

func (i *inmemoryClient) Query(ctx context.Context, in *api.Request,
	_ ...grpc.CallOption) (*api.Response, error) {
	return i.srv.Query(ctx, in)
}

func (i *inmemoryClient) Mutate(ctx context.Context, in *api.Mutation,
	_ ...grpc.CallOption) (*api.Assigned, error) {
	return i.srv.Mutate(ctx, in)
}

func (i *inmemoryClient) Alter(ctx context.Context, in *api.Operation,
	_ ...grpc.CallOption) (*api.Payload, error) {
	return i.srv.Alter(ctx, in)
}

func (i *inmemoryClient) CommitOrAbort(ctx context.Context, in *api.TxnContext,
	_ ...grpc.CallOption) (*api.TxnContext, error) {
	return i.srv.CommitOrAbort(ctx, in)
}

func (i *inmemoryClient) CheckVersion(ctx context.Context, in *api.Check,
	_ ...grpc.CallOption) (*api.Version, error) {
	return i.srv.CheckVersion(ctx, in)
}
