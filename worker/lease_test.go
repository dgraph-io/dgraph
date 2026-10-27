//go:build integration

/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/testutil"
)

// TestBumpLeaseIsIdempotent checks that bumping a lease raises it to the asked value once, and that
// bumping again to the same or a lower value leaves it where it is. A restore calls BumpLease with
// the backup's max uid, so restoring the same backup twice must not consume more ids.
func TestBumpLeaseIsIdempotent(t *testing.T) {
	conn, err := grpc.NewClient(testutil.GetSockAddrZero(),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer func() { require.NoError(t, conn.Close()) }()
	zc := pb.NewZeroClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// nextUid leases a single uid, which tells us where the lease currently stands.
	nextUid := func() uint64 {
		res, err := zc.AssignIds(ctx, &pb.Num{Val: 1, Type: pb.Num_UID})
		require.NoError(t, err)
		return res.GetStartId()
	}

	// Anything else in the cluster may lease uids while this runs, so the checks allow some slack
	// rather than expecting exact values. Leasing val more ids instead of bumping would move the
	// lease by at least target on every call, far past that slack.
	const slack = 100
	target := nextUid() + 1000
	require.NoError(t, BumpLease(ctx, zc, target, pb.Num_UID))
	first := nextUid()
	require.Greater(t, first, target)
	require.Less(t, first, target+slack)

	// Bumping again to target, or below it, must not move the lease.
	require.NoError(t, BumpLease(ctx, zc, target, pb.Num_UID))
	require.NoError(t, BumpLease(ctx, zc, target-500, pb.Num_UID))
	second := nextUid()
	require.Greater(t, second, first)
	require.Less(t, second, first+slack)
}
