//go:build integration2

/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
// advances startTxnTs past every previously-leased start timestamp, so committing the now-old
// transaction aborts
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

	// Open a transaction and mutate so it gets a real (soon-to-be-stale) start timestamp and keys.
	txn := gc.NewTxn()
	resp, err := txn.Mutate(ctx, &api.Mutation{SetJson: []byte(`{"name": "Manish"}`)})
	require.NoError(t, err)
	tc := resp.GetTxn()
	require.NotZero(t, tc.GetStartTs(), "mutation must yield a start ts")

	// Restart Zero. On coming back up it renews its lease and sets startTxnTs to MaxTxnTs+1,
	// which is strictly greater than the start timestamp leased above — making our open
	// transaction stale.
	require.NoError(t, c.StopZero(0))
	require.NoError(t, c.StartZero(0))
	require.NoError(t, c.HealthCheck(false))

	port, err := c.GetAlphaGrpcPublicPort(0)
	require.NoError(t, err)
	conn, err := grpc.NewClient("0.0.0.0:"+port, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	raw := api.NewDgraphClient(conn)

	// Commit the stale transaction via the raw stub to observe the categorized status. Zero having
	// a leader again is not enough: until the Alpha's own membership view catches up, its
	// CommitOrAbort hook has no Zero connection and fails with codes.Unknown. Poll the commit
	// itself until it reaches Zero and comes back Aborted. Retrying is safe because the stale
	// verdict does not change once startTxnTs has moved past this start ts.
	var st *status.Status
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		_, err := raw.CommitOrAbort(ctx, tc)
		st = status.Convert(err)
		assert.Equal(collect, codes.Aborted, st.Code(),
			"committing a txn whose start ts predates the new leader must abort; got %v", st)
	}, 60*time.Second, time.Second)

	require.True(t, strings.HasPrefix(st.Message(), "stale-startts: "),
		"abort reason should be categorized as stale-startts; got %q", st.Message())
}
