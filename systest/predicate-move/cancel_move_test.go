//go:build integration2

/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/dgraph-io/dgraph/v25/dgraphtest"
)

// cancelPayloadBytes is enough data that the move has real streaming work to do, and small enough
// that the retry after the cancellation completes in seconds.
const cancelPayloadBytes = 32 << 20

// TestCancelPredicateMove exercises Zero's /cancelMove endpoint against a real two-group cluster.
// The destination Alpha is paused (frozen with its sockets intact) so the move stalls mid-stream
// and stays cancellable for as long as the test needs, independent of data volume or host speed:
//
//  1. Load a payload into one predicate, pause the destination Alpha, and start a move.
//  2. Cancel it. The move must fail reporting the operator cancellation, the tablet must stay on
//     the source group, and commits on the predicate must flow again.
//  3. Unpause the destination and retry. The move must complete with the data intact, proving the
//     aborted attempt left nothing behind that a retry cannot clean up.
func TestCancelPredicateMove(t *testing.T) {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(2).WithNumZeros(1).WithReplicas(1)
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()
	require.NoError(t, c.Start())

	hc, err := c.HTTPClient()
	require.NoError(t, err)
	gc, cleanup, err := c.Client()
	require.NoError(t, err)
	defer cleanup()

	require.NoError(t, gc.DropAll())
	require.NoError(t, gc.SetupSchema(predicate+`: string .`))
	triples := loadPayload(t, gc, cancelPayloadBytes)

	_, srcGroup := waitForTablet(t, hc, time.Minute)
	state := membershipState(t, hc)
	dstGroup := otherGroup(t, state, srcGroup)
	srcAlpha := alphaInGroup(t, state, srcGroup)
	dstAlpha := alphaInGroup(t, state, dstGroup)
	t.Logf("moving group %d (alpha%d) -> group %d (alpha%d)", srcGroup, srcAlpha, dstGroup, dstAlpha)
	wantCount := countPayload(t, c, srcAlpha)
	require.Equal(t, triples, wantCount, "loaded triple count must be queryable before the move")

	// While the destination is frozen, talk only to the source Alpha and to Zero: the cluster's
	// default HTTP client may well point at the frozen Alpha.
	srcHC, err := c.GetAlphaHttpClient(srcAlpha)
	require.NoError(t, err)
	srcGC, srcCleanup, err := c.AlphaClient(srcAlpha)
	require.NoError(t, err)
	defer srcCleanup()

	require.NoError(t, c.PauseAlpha(dstAlpha))
	paused := true
	defer func() {
		if paused {
			_ = c.UnpauseAlpha(dstAlpha)
		}
	}()

	moveErrCh := make(chan error, 1)
	go func() { moveErrCh <- hc.MoveTablet(predicate, dstGroup) }()

	// Zero acknowledges a cancellation only for a move it is driving, so a successful cancel
	// proves the move was in flight.
	for deadline := time.Now().Add(2 * time.Minute); ; {
		err := hc.CancelMove(predicate)
		if err == nil {
			break
		}
		select {
		case moveErr := <-moveErrCh:
			t.Fatalf("move returned before it could be cancelled: %v", moveErr)
		default:
		}
		require.False(t, time.Now().After(deadline), "Zero never registered the move; last: %v", err)
		time.Sleep(200 * time.Millisecond)
	}

	select {
	case err := <-moveErrCh:
		require.Error(t, err, "a cancelled move must fail")
		require.ErrorContains(t, err, "cancelled by operator")
		t.Logf("move failed as expected: %v", err)
	case <-time.After(2 * time.Minute):
		t.Fatal("move did not unwind within 2m of being cancelled")
	}
	require.Error(t, hc.CancelMove(predicate), "nothing is in flight after the cancellation")

	// The tablet stays on the source group.
	_, group := findTablet(t, srcHC)
	require.Equal(t, srcGroup, group, "cancelled move must leave the tablet on the source group")

	// Unpause the destination before writing. While it was frozen, a write through the source
	// Alpha hung until the client deadline in an earlier version of this test: a hung peer appears
	// to stall transactions beyond its own group, which deserves its own investigation.
	require.NoError(t, c.UnpauseAlpha(dstAlpha))
	paused = false
	waitForHealth(t, c, 2*time.Minute)

	// Commits on the predicate flow again. Zero aborts every commit touching a predicate for as
	// long as its move is in progress, so a still-blocked predicate would fail this write outright.
	_, err = srcGC.Mutate(&api.Mutation{
		SetNquads: []byte(`_:n <` + predicate + `> "written after the cancelled move" .`),
		CommitNow: true,
	})
	require.NoError(t, err, "commits on the predicate must succeed once the move is cancelled")
	wantCount++
	require.Equal(t, wantCount, countPayload(t, c, srcAlpha), "data must be intact after a cancelled move")

	// Recovery: retry the move.
	retryMove(t, hc, dstGroup, 5*time.Minute)
	waitForTabletGroup(t, hc, dstGroup, 2*time.Minute)
	require.Equal(t, wantCount, countPayload(t, c, dstAlpha), "data must be intact on the destination group")
}
