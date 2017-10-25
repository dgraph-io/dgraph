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
package dgraph

import (
	"golang.org/x/net/context"

	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/protos"
)

// inmemoryClient implements protos.DgraphClient (it's equivalent of default grpc client, but for
// in-memory communication (without data (un)marshaling)).
type inmemoryClient struct {
	srv *Server
}

func (i *inmemoryClient) Query(ctx context.Context, in *protos.Request,
	_ ...grpc.CallOption) (*protos.Response, error) {
	return i.srv.Query(ctx, in)
}

func (i *inmemoryClient) Mutate(ctx context.Context, in *protos.Mutation,
	_ ...grpc.CallOption) (*protos.Assigned, error) {
	return i.srv.Mutate(ctx, in)
}

func (i *inmemoryClient) Alter(ctx context.Context, in *protos.Operation,
	_ ...grpc.CallOption) (*protos.Payload, error) {
	return i.srv.Alter(ctx, in)
}

func (i *inmemoryClient) CommitOrAbort(ctx context.Context, in *protos.TxnContext,
	_ ...grpc.CallOption) (*protos.Payload, error) {
	return i.srv.CommitOrAbort(ctx, in)
}

func (i *inmemoryClient) CheckVersion(ctx context.Context, in *protos.Check,
	_ ...grpc.CallOption) (*protos.Version, error) {
	return i.srv.CheckVersion(ctx, in)
}

func (i *inmemoryClient) AssignUids(ctx context.Context, in *protos.Num,
	_ ...grpc.CallOption) (*protos.AssignedIds, error) {
	return i.srv.AssignUids(ctx, in)
}
