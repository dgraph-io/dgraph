//go:build largemove

/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

// Long-running, manually-invoked integration test for large predicate moves. It is deliberately
// excluded from CI via the `largemove` build tag: it loads several GiB of data and can run for a
// long time. See README.md in this directory for how to run it.
package main

import (
	"os"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/dgraphapi"
	"github.com/dgraph-io/dgraph/v25/dgraphtest"
	"github.com/dgraph-io/dgraph/v25/protos/pb"
)

const (
	// minSizeForScaling is the tablet size Zero must report before the move is triggered.
	// Above ~1.8GiB the computed move timeout exceeds the 2h floor; 3GiB gives clear margin
	// (3GiB / 256KiB-per-second is about 3.4h).
	minSizeForScaling = 3 << 30

	// killAfter must exceed Zero's moveFailureMinElapsed (1m) so the induced failure registers
	// in the rebalancer backoff, and must be shorter than the move itself so the kill lands
	// mid-stream.
	killAfter = 75 * time.Second
)

// TestLargePredicateMove exercises the size-aware move timeout and the rebalancer backoff from
// dgraph-io/dgraph#9792 against a real two-group cluster:
//
//  1. Load MOVE_TEST_GB (default 8) GiB of incompressible data into one predicate and wait until
//     Zero reports the tablet size.
//  2. Trigger a move and kill the destination Alpha mid-stream. The move must fail, and Zero must
//     record the rebalancer backoff for the tablet.
//  3. Restart the destination and retry. The move must complete, both move announcements must
//     carry a size-scaled timeout (> 2h), and the data must be intact on the destination group.
func TestLargePredicateMove(t *testing.T) {
	gib := int64(8)
	if v := os.Getenv("MOVE_TEST_GB"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		require.NoError(t, err, "MOVE_TEST_GB must be an integer")
		require.Greater(t, n, int64(2), "MOVE_TEST_GB must be at least 3 for the timeout to scale")
		gib = n
	}
	targetBytes := gib << 30

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

	// Phase 1: load data.
	start := time.Now()
	t.Logf("loading %d GiB into predicate %q", gib, predicate)
	triples := loadPayload(t, gc, targetBytes)
	loadDur := time.Since(start)
	t.Logf("loaded %d triples (%d GiB) in %v", triples, gib, loadDur.Round(time.Second))

	// The induced failure below needs the move stream to outlast the kill point. A move cannot
	// run faster than ingest on the same host (it re-reads, rolls up, streams, and re-applies
	// every byte through the destination's Raft), so requiring bytes/ingestRate >= 2*killAfter
	// guarantees the kill lands mid-stream. Top up the data if the host is too fast for the
	// requested size.
	loadRate := float64(targetBytes) / loadDur.Seconds()
	if requiredBytes := int64(loadRate * 2 * killAfter.Seconds()); requiredBytes > targetBytes {
		extra := requiredBytes - targetBytes
		t.Logf("host ingests %.0f MiB/s; topping up %.1f GiB so the move outlasts the %v kill point",
			loadRate/(1<<20), float64(extra)/(1<<30), killAfter)
		triples += loadPayload(t, gc, extra)
		targetBytes = requiredBytes
	}

	// Wait for Zero to learn the tablet size. Alphas recompute tablet sizes on a periodic
	// ticker, so this can take several minutes after the load finishes.
	tab, srcGroup := waitForTabletSize(t, hc, minSizeForScaling, 20*time.Minute)
	t.Logf("Zero reports tablet size: ondisk=%d uncompressed=%d, served by group %d",
		tab.OnDiskBytes, tab.UncompressedBytes, srcGroup)

	state := membershipState(t, hc)
	dstGroup := otherGroup(t, state, srcGroup)
	srcAlpha := alphaInGroup(t, state, srcGroup)
	dstAlpha := alphaInGroup(t, state, dstGroup)
	t.Logf("moving group %d (alpha%d) -> group %d (alpha%d)", srcGroup, srcAlpha, dstGroup, dstAlpha)

	wantCount := countPayload(t, c, srcAlpha)
	require.Equal(t, triples, wantCount, "loaded triple count must be queryable before the move")

	// Phase 2: induced failure. Kill the destination Alpha mid-stream; the move must fail and
	// Zero must record the rebalancer backoff. The kill lands after moveFailureMinElapsed (1m)
	// so the failure is treated as expensive.
	moveErrCh := make(chan error, 1)
	go func() { moveErrCh <- hc.MoveTablet(predicate, dstGroup) }()
	select {
	case err := <-moveErrCh:
		t.Fatalf("move finished before the %v kill point (err=%v); the move ran faster than the"+
			" measured ingest rate predicted, increase MOVE_TEST_GB", killAfter, err)
	case <-time.After(killAfter):
	}
	t.Logf("killing destination alpha%d mid-move", dstAlpha)
	require.NoError(t, c.KillAlpha(dstAlpha))
	select {
	case err := <-moveErrCh:
		require.Error(t, err, "move must fail after the destination died")
		t.Logf("move failed as expected: %v", err)
	case <-time.After(10 * time.Minute):
		t.Fatal("move did not return within 10m of the destination dying")
	}
	require.NoError(t,
		c.WaitForAnyZeroLog("Skipping automatic rebalancing of this tablet", 2*time.Minute, 5*time.Second),
		"Zero must record the rebalancer backoff after an expensive failed move")

	// Phase 3: recovery. Restart the destination and retry until the move goes through. Early
	// retries may fail while Zero reconnects to the restarted group; those quick failures are
	// expected and do not feed the backoff. State assertions about the failed move run after
	// health returns: the killed alpha may be the very one serving the cluster's HTTP client,
	// so poking /state during the outage would test our luck, not the move.
	require.NoError(t, c.StartAlpha(dstAlpha))
	// The killed Alpha replays its WAL and badger state on restart (it absorbed several GiB
	// mid-move before dying), and re-establishes cluster connections; health can take minutes
	// to come back.
	waitForHealth(t, c, 10*time.Minute)

	// The failed move must have left the tablet on the source group, with all data intact.
	_, group := findTablet(t, hc)
	require.Equal(t, srcGroup, group, "failed move must leave the tablet on the source group")
	require.Equal(t, wantCount, countPayload(t, c, srcAlpha), "data must be intact after a failed move")
	retryMove(t, hc, dstGroup, 90*time.Minute)
	waitForTabletGroup(t, hc, dstGroup, 2*time.Minute)
	require.Equal(t, wantCount, countPayload(t, c, dstAlpha), "data must be intact on the destination group")

	// Both move attempts ran after Zero learned the tablet size, so both announcements must
	// carry a size-scaled timeout above the 2h floor.
	logs, err := c.GetZeroLogs(0)
	require.NoError(t, err)
	moveLine := regexp.MustCompile(`Going to move predicate: \[(?:0-)?` + predicate + `\][^\n]*timeout: (\S+)`)
	matches := moveLine.FindAllStringSubmatch(logs, -1)
	require.GreaterOrEqual(t, len(matches), 2, "expected move announcements for both attempts in Zero logs")
	for _, m := range matches {
		d, err := time.ParseDuration(m[1])
		require.NoError(t, err, "move announcement must include a parseable timeout: %q", m[0])
		require.Greater(t, d, 2*time.Hour, "announced move timeout must be size-scaled above the 2h floor")
	}
	t.Logf("all %d move announcements carried a size-scaled timeout (last: %s)",
		len(matches), matches[len(matches)-1][1])
}

// waitForTabletSize polls the membership state until Zero reports the payload tablet at or above
// minBytes (using the larger of on-disk and uncompressed size, mirroring Zero's moveTimeout).
func waitForTabletSize(t *testing.T, hc *dgraphapi.HTTPClient, minBytes int64,
	timeout time.Duration) (*pb.Tablet, uint32) {
	deadline := time.Now().Add(timeout)
	for {
		tab, gid := findTablet(t, hc)
		size := max(tab.OnDiskBytes, tab.UncompressedBytes)
		if size >= minBytes {
			return tab, gid
		}
		require.False(t, time.Now().After(deadline),
			"Zero did not report tablet size >= %d within %v (last: ondisk=%d uncompressed=%d);"+
				" tablet sizes are recomputed periodically, or the data may be too small",
			minBytes, timeout, tab.OnDiskBytes, tab.UncompressedBytes)
		t.Logf("waiting for Zero to report tablet size (ondisk=%d uncompressed=%d, want >= %d)",
			tab.OnDiskBytes, tab.UncompressedBytes, minBytes)
		time.Sleep(30 * time.Second)
	}
}
