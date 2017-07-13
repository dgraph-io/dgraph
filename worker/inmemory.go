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
package worker

import (
	"golang.org/x/net/context"

	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/protos"
)

// inmemoryWorker implements protos.WorkerClient gRPC interface
type inmemoryWorker struct {
	srv protos.WorkerServer
}

func newInmemoryWorker() *inmemoryWorker {
	return &inmemoryWorker{&grpcWorker{}}
}

func (i *inmemoryWorker) Echo(ctx context.Context, in *protos.Payload,
	_ ...grpc.CallOption) (*protos.Payload, error) {
	return i.srv.Echo(ctx, in)
}

func (i *inmemoryWorker) AssignUids(ctx context.Context, in *protos.Num,
	_ ...grpc.CallOption) (*protos.AssignedIds, error) {
	return i.srv.AssignUids(ctx, in)
}

func (i *inmemoryWorker) Mutate(ctx context.Context, in *protos.Mutations,
	_ ...grpc.CallOption) (*protos.Payload, error) {
	return i.srv.Mutate(ctx, in)
}

func (i *inmemoryWorker) ServeTask(ctx context.Context, in *protos.Query,
	_ ...grpc.CallOption) (*protos.Result, error) {
	return i.srv.ServeTask(ctx, in)
}

// This function is never called
func (i *inmemoryWorker) PredicateAndSchemaData(ctx context.Context,
	_ ...grpc.CallOption) (protos.Worker_PredicateAndSchemaDataClient, error) {
	panic("not implemented")
}

func (i *inmemoryWorker) Sort(ctx context.Context, in *protos.SortMessage,
	_ ...grpc.CallOption) (*protos.SortResult, error) {
	return i.srv.Sort(ctx, in)
}

func (i *inmemoryWorker) Schema(ctx context.Context, in *protos.SchemaRequest,
	_ ...grpc.CallOption) (*protos.SchemaResult, error) {
	return i.srv.Schema(ctx, in)
}

func (i *inmemoryWorker) RaftMessage(ctx context.Context, in *protos.Payload,
	_ ...grpc.CallOption) (*protos.Payload, error) {
	return i.srv.RaftMessage(ctx, in)
}

func (i *inmemoryWorker) JoinCluster(ctx context.Context, in *protos.RaftContext,
	_ ...grpc.CallOption) (*protos.Payload, error) {
	return i.srv.JoinCluster(ctx, in)
}

func (i *inmemoryWorker) UpdateMembership(ctx context.Context, in *protos.MembershipUpdate,
	_ ...grpc.CallOption) (*protos.MembershipUpdate, error) {
	return i.srv.UpdateMembership(ctx, in)
}

func (i *inmemoryWorker) Export(ctx context.Context, in *protos.ExportPayload,
	_ ...grpc.CallOption) (*protos.ExportPayload, error) {
	return i.srv.Export(ctx, in)
}
