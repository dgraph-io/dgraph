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
package embedded

import (
	"golang.org/x/net/context"

	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/server"
)

type EmbeddedDgraph interface {
	protos.DgraphClient
}

type inmemoryClient struct {
	srv *server.GrpcServer
}

func (i *inmemoryClient) Run(ctx context.Context, in *protos.Request, opts ...grpc.CallOption) (*protos.Response, error) {
	return i.srv.Run(ctx, in)
}

func (i *inmemoryClient) CheckVersion(ctx context.Context, in *protos.Check, opts ...grpc.CallOption) (*protos.Version, error) {
	return i.srv.CheckVersion(ctx, in)
}

func (i *inmemoryClient) AssignUids(ctx context.Context, in *protos.Num, opts ...grpc.CallOption) (*protos.AssignedIds, error) {
	return i.srv.AssignUids(ctx, in)
}

func NewEmbeddedDgraph() EmbeddedDgraph {
	// TODO(tzdybal) - create embedded server backend
	embedded := inmemoryClient{&server.GrpcServer{}}

	return &embedded
}

// NewBatchMutation is used to create a new batch.
// size is the number of RDF's that are sent as part of one request to Dgraph.
// pending is the number of concurrent requests to make to Dgraph server.
func NewEmbeddedDgraphClient(embedded EmbeddedDgraph, opts client.BatchMutationOptions) *client.Dgraph {
	return client.NewGenericClient(embedded, opts)
}
