//go:build integration2

/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/dgraph-io/dgraph/v25/dgraphtest"
)

// TestStaleStartTsAbortReason proves the "stale-startts" abort category end-to-end against a
// real cluster. A transaction's start timestamp becomes "stale" when it predates the current
// Zero leader's lease — i.e. after a leader change. We reproduce that deterministically by
// opening a transaction, then restarting the (single) Zero: on restart Zero renews its lease and
// advances startTxnTs past every previously-leased start ts, so committing the now-old txn aborts
// with the stale-startts reason rather than a plain write-write conflict.
//
// As in the alpha-level reason tests, the commit goes through the raw CommitOrAbort stub so we
// observe the unflattened codes.Aborted status (dgo's Txn.Commit would replace it with the
// reasonless ErrAborted).
func TestStaleStartTsAbortReason(t *testing.T) {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1)
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	t.Cleanup(func() { c.Cleanup(t.Failed()) })
	require.NoError(t, c.Start())

	gc, cleanup, err := c.Client()
	require.NoError(t, err)
	defer cleanup()

	ctx := context.Background()
	require.NoError(t, gc.Alter(ctx, &api.Operation{DropAll: true}))

	// Open a transaction and mutate so it gets a real (soon-to-be-stale) start ts and keys.
	txn := gc.NewTxn()
	resp, err := txn.Mutate(ctx, &api.Mutation{SetJson: []byte(`{"name": "Manish"}`)})
	require.NoError(t, err)
	tc := resp.GetTxn()
	require.NotZero(t, tc.GetStartTs(), "mutation must yield a start ts")

	// Restart Zero. On coming back up it renews its lease and sets startTxnTs to MaxTxnTs+1,
	// which is strictly greater than the start ts leased above — making our open txn stale.
	require.NoError(t, c.StopZero(0))
	require.NoError(t, c.StartZero(0))
	require.NoError(t, c.HealthCheck(false))

	// Wait until a Zero leader is established again (lease renewal, hence startTxnTs bump, runs
	// when a Zero becomes leader). Avoids racing the commit against the leaderless window.
	require.Eventually(t, func() bool {
		_, err := c.GetZeroLeader(0)
		return err == nil
	}, 60*time.Second, time.Second, "zero leader did not re-establish after restart")

	// Commit the stale txn via the raw stub to observe the categorized status.
	port, err := c.GetAlphaGrpcPublicPort(0)
	require.NoError(t, err)
	conn, err := grpc.NewClient("0.0.0.0:"+port, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	raw := api.NewDgraphClient(conn)

	_, err = raw.CommitOrAbort(ctx, tc)
	require.Error(t, err, "committing a txn whose start ts predates the new leader must abort")

	st := status.Convert(err)
	require.Equal(t, codes.Aborted, st.Code(),
		"abort must keep codes.Aborted so existing clients still retry")
	require.True(t, strings.HasPrefix(st.Message(), "stale-startts: "),
		"abort reason should be categorized as stale-startts; got %q", st.Message())
}
