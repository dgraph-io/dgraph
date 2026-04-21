//go:build integration2

/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/dgraphtest"
)

// These tests verify that Dgraph Zero reconciles stale addresses that were
// baked into its Raft WAL at initial bootstrap. Each test targets one of the
// scenarios identified in the design:
//
//   1.  Leader changes address, reconciles itself (single-Zero cluster).
//   2.  WAL replay — address survives a subsequent restart.
//   4.  Leader changes address in multi-Zero cluster; followers apply.
//   5.  Follower changes address; leader detects via Raft transport peer map.
//   6.  Multiple Zeros change addresses simultaneously; sequential reconcile.
//   7.  Old leader changes address after losing leadership; new leader fixes.
//   9.  Follower restart: its own address converges for the Connect response.
//  11.  Duplicate address is rejected by the uniqueness check.
//  12.  Restart with unchanged --my triggers no proposal and no state change.
//
// Numbering follows the scenarios enumerated in the design discussion.
//
// Positive convergence is asserted via /state polling. "Negative" assertions
// (no proposal, duplicate rejected) are anchored to concrete log markers
// emitted by the fix instead of sleeping for a fixed duration, which keeps
// the suite fast and avoids flakes on slow CI.

const (
	reconcileTimeout      = 30 * time.Second
	multiReconcileTimeout = 120 * time.Second
	reconcilePoll         = time.Second

	// Log markers emitted by the Zero reconciliation code path. Keep these in
	// sync with glog statements in dgraph/cmd/zero/raft.go.
	logApplied      = "Applied ConfChangeUpdateNode"
	logProposed     = "Proposing ConfChangeUpdateNode"
	logBecameLeader = "I've become the leader"
	// logReconcileComplete is emitted at V(2) at the end of reconcileZeroAddresses
	// when the loop completes without making any proposal. It is used for negative
	// assertions (e.g. "no change committed", "duplicate rejected") where /state
	// polling alone is insufficient: the /state endpoint requires WaitLinearizableRead
	// and is unavailable during leadership transitions, making it impossible to
	// distinguish "address unchanged" from "endpoint temporarily unreachable".
	// Test containers run with -v=2 so V(2) logs are visible in container output.
	logReconcileComplete = "Zero address reconciliation complete: all addresses up to date"
)

// waitForAddr asserts a Zero's address converges within reconcileTimeout.
func waitForAddr(t *testing.T, c *dgraphtest.LocalCluster, queryIdx int, raftID, want string) {
	t.Helper()
	addr, err := c.WaitForZeroAddress(queryIdx, raftID, want, reconcileTimeout, reconcilePoll)
	require.NoError(t, err, "zero %s did not converge to %q on zero%d (last seen %q)",
		raftID, want, queryIdx, addr)
}

// waitForAddrLong is the same as waitForAddr but with a longer deadline for
// scenarios that must span multiple reconciliation cycles.
func waitForAddrLong(t *testing.T, c *dgraphtest.LocalCluster, queryIdx int, raftID, want string) {
	t.Helper()
	addr, err := c.WaitForZeroAddress(queryIdx, raftID, want, multiReconcileTimeout, reconcilePoll)
	require.NoError(t, err, "zero %s did not converge to %q on zero%d (last seen %q)",
		raftID, want, queryIdx, addr)
}

// waitForZeroLog asserts a log marker appears on the given Zero within reconcileTimeout.
func waitForZeroLog(t *testing.T, c *dgraphtest.LocalCluster, id int, substr string) {
	t.Helper()
	require.NoError(t,
		c.WaitForZeroLog(id, substr, reconcileTimeout, reconcilePoll),
		"expected log marker %q on zero%d", substr, id)
}

// requireNoLog asserts the given marker is not present in the Zero's logs.
func requireNoLog(t *testing.T, c *dgraphtest.LocalCluster, id int, substr string) {
	t.Helper()
	found, err := c.ZeroLogContains(id, substr)
	require.NoError(t, err)
	require.False(t, found, "unexpected log marker %q on zero%d", substr, id)
}

// waitForAllZeros polls /state on zero0 until all numZeros members are visible,
// ensuring the Raft group has fully formed before tests run. Required for
// multi-Zero tests that call GetZeroLeader/GetZeroFollower immediately after Start().
func waitForAllZeros(t *testing.T, c *dgraphtest.LocalCluster, numZeros int) {
	t.Helper()
	require.Eventually(t, func() bool {
		state, err := c.GetZeroState(0)
		if err != nil {
			return false
		}
		return len(state.Zeros) == numZeros
	}, reconcileTimeout, reconcilePoll,
		"expected %d zeros in /state, cluster did not stabilize", numZeros)
}

func newSingleZeroCluster(t *testing.T) *dgraphtest.LocalCluster {
	t.Helper()
	return newCluster(t, 1)
}

func newMultiZeroCluster(t *testing.T) *dgraphtest.LocalCluster {
	t.Helper()
	return newCluster(t, 3)
}

func newCluster(t *testing.T, numZeros int) *dgraphtest.LocalCluster {
	t.Helper()
	conf := dgraphtest.NewClusterConfig().
		WithNumAlphas(1).
		WithNumZeros(numZeros).
		WithReplicas(1)
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	t.Cleanup(func() { c.Cleanup(t.Failed()) })
	require.NoError(t, c.Start())
	if numZeros > 1 {
		waitForAllZeros(t, c, numZeros)
	}
	return c
}

// ---------------------------------------------------------------------------
// Scenario 1: Leader changes address, reconciles itself (single Zero).
// ---------------------------------------------------------------------------

func TestZeroAddr_LeaderSelfReconcile(t *testing.T) {
	c := newSingleZeroCluster(t)

	leader, err := c.GetZeroLeader(0)
	require.NoError(t, err)

	require.NoError(t, c.StopAlpha(0))
	newAddr, err := c.ChangeZeroAddress(leader.ContainerIdx)
	require.NoError(t, err)
	require.NoError(t, c.HealthCheck(true))

	waitForAddr(t, c, 0, leader.RaftID, newAddr)

	require.NoError(t, c.StartAlpha(0))
	require.NoError(t, c.HealthCheck(false))
}

// ---------------------------------------------------------------------------
// Scenario 2: WAL replay — address survives a subsequent restart.
// ---------------------------------------------------------------------------

func TestZeroAddr_SurvivesRestart(t *testing.T) {
	c := newSingleZeroCluster(t)

	leader, err := c.GetZeroLeader(0)
	require.NoError(t, err)

	require.NoError(t, c.StopAlpha(0))

	// First restart: reconcile to a new address. The log marker proves the
	// ConfChangeUpdateNode was both proposed and applied.
	newAddr, err := c.ChangeZeroAddress(leader.ContainerIdx)
	require.NoError(t, err)
	require.NoError(t, c.HealthCheck(true))
	waitForAddr(t, c, 0, leader.RaftID, newAddr)
	waitForZeroLog(t, c, leader.ContainerIdx, logApplied)

	// Second restart with the SAME --my. The WAL now carries the
	// ConfChangeUpdateNode from the previous run; replay alone must restore
	// the reconciled address. Poll /state immediately without sleeping —
	// the address must already be correct on the first successful read.
	require.NoError(t, c.StopZero(leader.ContainerIdx))
	require.NoError(t, c.RecreateZero(leader.ContainerIdx))
	require.NoError(t, c.StartZero(leader.ContainerIdx))
	require.NoError(t, c.HealthCheck(true))
	waitForAddr(t, c, 0, leader.RaftID, newAddr)

	require.NoError(t, c.StartAlpha(0))
	require.NoError(t, c.HealthCheck(false))
}

// ---------------------------------------------------------------------------
// Scenario 4: Leader changes address in multi-Zero cluster.
// ---------------------------------------------------------------------------

func TestZeroAddr_LeaderChangeMultiZero(t *testing.T) {
	c := newMultiZeroCluster(t)

	leader, err := c.GetZeroLeader(0)
	require.NoError(t, err)
	follower, err := c.GetZeroFollower(0)
	require.NoError(t, err)

	newAddr, err := c.ChangeZeroAddress(leader.ContainerIdx)
	require.NoError(t, err)

	// The restarted leader reconciles its own address; the ConfChangeUpdateNode
	// is replicated to all followers which apply it deterministically.
	// Convergence of /state on both nodes proves proposal and application.
	waitForAddr(t, c, leader.ContainerIdx, leader.RaftID, newAddr)
	waitForAddr(t, c, follower.ContainerIdx, leader.RaftID, newAddr)
}

// ---------------------------------------------------------------------------
// Scenario 5: Follower changes address, leader reconciles via peer map.
// ---------------------------------------------------------------------------

func TestZeroAddr_FollowerChangeMultiZero(t *testing.T) {
	c := newMultiZeroCluster(t)

	follower, err := c.GetZeroFollower(0)
	require.NoError(t, err)
	leader, err := c.GetZeroLeader(0)
	require.NoError(t, err)

	newAddr, err := c.ChangeZeroAddress(follower.ContainerIdx)
	require.NoError(t, err)

	// Poll /state on both nodes until the new address is committed. This
	// covers the full path: follower reconnects → leader detects mismatch
	// via transport peer map → proposes ConfChangeUpdateNode → all nodes apply.
	waitForAddrLong(t, c, leader.ContainerIdx, follower.RaftID, newAddr)
	waitForAddrLong(t, c, follower.ContainerIdx, follower.RaftID, newAddr)
}

// ---------------------------------------------------------------------------
// Scenario 6: Two followers change addresses simultaneously.
// ---------------------------------------------------------------------------

func TestZeroAddr_MultipleFollowersChange(t *testing.T) {
	c := newMultiZeroCluster(t)

	leader, err := c.GetZeroLeader(0)
	require.NoError(t, err)
	followers, err := c.GetZeroFollowers(0)
	require.NoError(t, err)
	require.Len(t, followers, 2, "expected 2 followers in a 3-Zero cluster")

	newAddrs := make([]string, len(followers))
	for i, f := range followers {
		addr, err := c.ChangeZeroAddress(f.ContainerIdx)
		require.NoError(t, err)
		newAddrs[i] = addr
	}

	// The leader proposes one ConfChangeUpdateNode per 10-second cycle
	// (Raft's single-pending-ConfChange constraint), so both addresses
	// should converge within the extended window.
	for i, f := range followers {
		waitForAddrLong(t, c, leader.ContainerIdx, f.RaftID, newAddrs[i])
	}
}

// ---------------------------------------------------------------------------
// Scenario 7: New leader reconciles stale address of the previous leader.
// ---------------------------------------------------------------------------

func TestZeroAddr_NewLeaderReconciles(t *testing.T) {
	c := newMultiZeroCluster(t)

	oldLeader, err := c.GetZeroLeader(0)
	require.NoError(t, err)
	require.NoError(t, c.StopZero(oldLeader.ContainerIdx))

	newLeaderIdx := waitForNewLeader(t, c, oldLeader.ContainerIdx, 3)
	t.Logf("new leader is zero%d", newLeaderIdx)

	// Restart the old leader with a new --my; it will rejoin as a follower.
	// The new leader must reconcile the stale address via its peer map.
	containerName, err := c.GetZeroContainerName(oldLeader.ContainerIdx)
	require.NoError(t, err)
	newAddr := containerName + ":5080"
	require.NoError(t, c.SetZeroMyAddr(oldLeader.ContainerIdx, newAddr))
	require.NoError(t, c.RecreateZero(oldLeader.ContainerIdx))
	require.NoError(t, c.StartZero(oldLeader.ContainerIdx))

	waitForAddr(t, c, newLeaderIdx, oldLeader.RaftID, newAddr)
}

func waitForNewLeader(t *testing.T, c *dgraphtest.LocalCluster, excludeIdx, numZeros int) int {
	t.Helper()
	newLeaderIdx := -1
	require.Eventually(t, func() bool {
		for i := range numZeros {
			if i == excludeIdx {
				continue
			}
			leader, err := c.GetZeroLeader(i)
			if err == nil && leader.ContainerIdx != excludeIdx {
				newLeaderIdx = leader.ContainerIdx
				return true
			}
		}
		return false
	}, reconcileTimeout, reconcilePoll, "no new leader elected after excluding zero%d", excludeIdx)
	return newLeaderIdx
}

// ---------------------------------------------------------------------------
// Scenario 9: Follower address converges in multi-Zero Connect flow.
// ---------------------------------------------------------------------------

func TestZeroAddr_FollowerConvergesOnSelf(t *testing.T) {
	c := newMultiZeroCluster(t)

	follower, err := c.GetZeroFollower(0)
	require.NoError(t, err)

	newAddr, err := c.ChangeZeroAddress(follower.ContainerIdx)
	require.NoError(t, err)

	// Regardless of which path corrects it (Raft commit or serving-layer
	// override), the follower's own /state must converge to --my.
	// Uses the long timeout: the detection path (leader sees reconnect via
	// transport peer map → 10s reconcile cycle → proposes ConfChange) is
	// identical to scenario 5, which also uses multiReconcileTimeout.
	waitForAddrLong(t, c, follower.ContainerIdx, follower.RaftID, newAddr)
}

// ---------------------------------------------------------------------------
// Scenario 11: Duplicate --my is rejected by the uniqueness check.
// ---------------------------------------------------------------------------

func TestZeroAddr_DuplicateRejected(t *testing.T) {
	c := newMultiZeroCluster(t)

	leader, err := c.GetZeroLeader(0)
	require.NoError(t, err)
	follower, err := c.GetZeroFollower(0)
	require.NoError(t, err)
	originalFollowerAddr := follower.Addr

	// Reconfigure the follower with the leader's address (deliberate collision).
	require.NoError(t, c.StopZero(follower.ContainerIdx))
	require.NoError(t, c.SetZeroMyAddr(follower.ContainerIdx, leader.Addr))
	require.NoError(t, c.RecreateZero(follower.ContainerIdx))
	require.NoError(t, c.StartZero(follower.ContainerIdx))

	// Ensure the follower has rejoined the Raft group before waiting for the
	// reconciliation log marker. Without this, logReconcileComplete could be
	// satisfied by a cycle that fired before the duplicate address was visible
	// to the leader's reconcile loop.
	waitForAllZeros(t, c, 3)

	// Wait for any Zero to complete a full reconciliation cycle. logReconcileComplete
	// is emitted at the end of reconcileZeroAddresses() when no proposal was made —
	// which covers both "all correct" and "duplicate skipped" cases. Its presence
	// proves that reconcile ran and intentionally left the address unchanged.
	require.NoError(t,
		c.WaitForAnyZeroLog(logReconcileComplete, multiReconcileTimeout, reconcilePoll),
		"expected log marker %q on any zero", logReconcileComplete)

	// Verify via /state that the duplicate was not committed. Try each Zero
	// in turn until one responds successfully.
	var state *dgraphtest.ZeroState
	for i := range 3 {
		if s, err := c.GetZeroState(i); err == nil {
			state = s
			break
		}
	}
	require.NotNil(t, state, "could not get /state from any zero")
	require.Equal(t, originalFollowerAddr, state.Zeros[follower.RaftID].Addr,
		"duplicate address must not overwrite existing member's address")
}

// ---------------------------------------------------------------------------
// Scenario 12: Restart with unchanged --my is a no-op.
// ---------------------------------------------------------------------------

func TestZeroAddr_NoChangeIsNoOp(t *testing.T) {
	c := newSingleZeroCluster(t)

	leader, err := c.GetZeroLeader(0)
	require.NoError(t, err)
	originalAddr := leader.Addr

	require.NoError(t, c.StopAlpha(0))
	require.NoError(t, c.StopZero(leader.ContainerIdx))
	require.NoError(t, c.RecreateZero(leader.ContainerIdx))
	require.NoError(t, c.StartZero(leader.ContainerIdx))
	require.NoError(t, c.HealthCheck(true))

	// Wait for leader election, then wait for the reconciliation loop to
	// complete at least one full cycle. The "complete" marker is emitted
	// only when the loop runs and finds no mismatches, so its presence
	// proves the loop ran — making the no-proposal assertion meaningful.
	waitForZeroLog(t, c, leader.ContainerIdx, logBecameLeader)
	waitForZeroLog(t, c, leader.ContainerIdx, logReconcileComplete)

	requireNoLog(t, c, leader.ContainerIdx, logProposed)
	requireNoLog(t, c, leader.ContainerIdx, logApplied)

	state, err := c.GetZeroState(0)
	require.NoError(t, err)
	require.Equal(t, originalAddr, state.Zeros[leader.RaftID].Addr,
		"address must not change when --my is unchanged")

	require.NoError(t, c.StartAlpha(0))
	require.NoError(t, c.HealthCheck(false))
}
