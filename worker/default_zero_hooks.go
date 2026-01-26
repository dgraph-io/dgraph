/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
	"context"

	"google.golang.org/grpc/metadata"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/dgraph-io/dgraph/v25/conn"
	"github.com/dgraph-io/dgraph/v25/hooks"
	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/x"
)

type defaultZeroHooks struct{}

func init() {
	hooks.SetDefaultZeroHooks(defaultZeroHooks{})
}

func (defaultZeroHooks) AssignUIDs(ctx context.Context, num *pb.Num) (*pb.AssignedIds, error) {
	if num.Type == 0 {
		num.Type = pb.Num_UID
	}

	// Pass on the incoming metadata to the zero. Namespace from the metadata is required by zero.
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		ctx = metadata.NewOutgoingContext(ctx, md)
	}

	pl := groups().Leader(0)
	if pl == nil {
		return nil, conn.ErrNoConnection
	}

	c := pb.NewZeroClient(pl.Get())
	return c.AssignIds(ctx, num)
}

func (defaultZeroHooks) AssignTimestamps(ctx context.Context, num *pb.Num) (*pb.AssignedIds, error) {
	pl := groups().connToZeroLeader()
	if pl == nil {
		return nil, conn.ErrNoConnection
	}

	c := pb.NewZeroClient(pl.Get())
	return c.Timestamps(ctx, num)
}

func (defaultZeroHooks) AssignNsIDs(ctx context.Context, num *pb.Num) (*pb.AssignedIds, error) {
	num.Type = pb.Num_NS_ID

	pl := groups().Leader(0)
	if pl == nil {
		return nil, conn.ErrNoConnection
	}

	c := pb.NewZeroClient(pl.Get())
	return c.AssignIds(ctx, num)
}

func (defaultZeroHooks) CommitOrAbort(ctx context.Context, tc *api.TxnContext) (*api.TxnContext, error) {
	pl := groups().Leader(0)
	if pl == nil {
		return nil, conn.ErrNoConnection
	}

	// Do de-duplication before sending the request to zero.
	tc.Keys = x.Unique(tc.Keys)
	tc.Preds = x.Unique(tc.Preds)

	zc := pb.NewZeroClient(pl.Get())
	return zc.CommitOrAbort(ctx, tc)
}

func (defaultZeroHooks) ApplyMutations(ctx context.Context, m *pb.Mutations) (*api.TxnContext, error) {
	if groups().ServesGroup(m.GroupId) {
		txnCtx := &api.TxnContext{}
		return txnCtx, (&grpcWorker{}).proposeAndWait(ctx, txnCtx, m)
	}

	pl := groups().Leader(m.GroupId)
	if pl == nil {
		return nil, conn.ErrNoConnection
	}

	var tc *api.TxnContext
	c := pb.NewWorkerClient(pl.Get())

	ch := make(chan error, 1)
	go func() {
		var err error
		tc, err = c.Mutate(ctx, m)
		ch <- err
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-ch:
		return tc, err
	}
}
