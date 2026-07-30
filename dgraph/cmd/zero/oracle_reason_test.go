/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package zero

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/dgraph-io/dgraph/v25/protos/pb"
)

// The abort-reason wire format is a contract with gRPC clients (e.g. dgraph4j parses the
// "<code>: " prefix into TxnConflictException.AbortReason). These unit tests pin the
// category prefixes and the logic that selects between them, so the contract can't drift
// silently without an integration cluster.

func TestAbortReasonFormat(t *testing.T) {
	require.Equal(t, "conflict: boom", abortReason(abortReasonConflict, "boom"))
	require.Equal(t, "stale-startts: x", abortReason(abortReasonStaleStartTs, "x"))
	require.Equal(t, "predicate-move: y", abortReason(abortReasonPredicateMove, "y"))
}

// TestAbortReasonUncategorized pins the withholding contract: when the server cannot substantiate
// one of the published categories it emits the bare detail with no prefix, rather than borrowing a
// category that would imply the wrong remedy. That is byte-identical to what a pre-feature server
// sends, and both dgraph4j and pydgraph already degrade a prefix-less message to UNKNOWN. The
// absence of a colon-delimited prefix is the whole contract, so it must not regress.
func TestAbortReasonUncategorized(t *testing.T) {
	require.Equal(t, "Tablet for foo is nil",
		abortReason(abortReasonUncategorized, "Tablet for foo is nil"))
	require.Equal(t, "context canceled", abortReason(abortReasonUncategorized, "context canceled"))

	// No withheld detail may begin with a token a client would parse as a real category, otherwise
	// withholding would be silently reinterpreted as a category.
	for _, detail := range []string{
		"Unable to find group id in 1foo",
		"Tablet for foo is nil",
		"context canceled",
		"context deadline exceeded",
	} {
		got := abortReason(abortReasonUncategorized, detail)
		prefix := got
		if i := strings.Index(got, ":"); i >= 0 {
			prefix = got[:i]
		}
		for _, code := range []string{
			abortReasonConflict, abortReasonStaleStartTs, abortReasonPredicateMove,
		} {
			require.NotEqual(t, code, strings.ToLower(strings.TrimSpace(prefix)),
				"withheld detail %q would be parsed as category %q", detail, code)
		}
	}
}

func TestConflictAbortReason(t *testing.T) {
	// Write-write conflict.
	r := conflictAbortReason(false)
	require.True(t, strings.HasPrefix(r, abortReasonConflict+": "),
		"want conflict prefix, got %q", r)
	require.Equal(t, abortReason(abortReasonConflict, abortDetailConflict), r)

	// Stale start timestamp. The detail must name both ways startTxnTs rises — a Zero leader change
	// and a conflict-map trim at a snapshot — because asserting only the first is wrong on the
	// second path (purgeBelow via applySnapshot, which involves no leader change at all).
	r = conflictAbortReason(true)
	require.True(t, strings.HasPrefix(r, abortReasonStaleStartTs+": "),
		"want stale-startts prefix, got %q", r)
	require.Equal(t, abortReason(abortReasonStaleStartTs, abortDetailStaleStartTs), r)
	require.Contains(t, r, "leader change")
	require.Contains(t, r, "snapshot")
}

// TestCheckPredsCategories is the core of the abort-category fix. checkPreds has five exits and only
// two of them are predicate moves; the other three are unrelated failures with different remedies,
// so they must not be reported as moves. Every case still returns an error (the transaction aborts
// regardless) — this pins only which category each cause claims.
func TestCheckPredsCategories(t *testing.T) {
	const pred = "friend"
	servingIn := func(gid uint32) *Server {
		s := &Server{}
		s.blockCommitsOn = new(sync.Map)
		s.state = &pb.MembershipState{Groups: map[uint32]*pb.Group{
			gid: {Tablets: map[string]*pb.Tablet{pred: {GroupId: gid, Predicate: pred}}},
		}}
		return s
	}

	t.Run("in-flight move is reported as a move", func(t *testing.T) {
		// The authoritative signal, and it must win even though the tablet still resolves cleanly.
		s := servingIn(1)
		s.blockCommitsOn.Store(pred, struct{}{})
		reason, err := s.checkPreds([]string{"1-" + pred})
		require.Error(t, err)
		require.Equal(t, abortReasonPredicateMove, reason)
		require.Contains(t, err.Error(), "blocked due to predicate move")
	})

	t.Run("in-flight move wins over an absent tablet", func(t *testing.T) {
		// Regression guard for the check ordering. blockTablet is held for the whole move, so if the
		// tablet lookup ran first a genuine in-flight move could be reported as uncategorized.
		s := &Server{state: &pb.MembershipState{Groups: map[uint32]*pb.Group{}}}
		s.blockCommitsOn = new(sync.Map)
		s.blockCommitsOn.Store(pred, struct{}{})
		reason, err := s.checkPreds([]string{"1-" + pred})
		require.Error(t, err)
		require.Equal(t, abortReasonPredicateMove, reason,
			"isBlocked must be consulted before the tablet lookup")
	})

	t.Run("completed move is reported as a move", func(t *testing.T) {
		// Written against group 1, but the predicate now belongs to group 2.
		reason, err := servingIn(2).checkPreds([]string{"1-" + pred})
		require.Error(t, err)
		require.Equal(t, abortReasonPredicateMove, reason)
		require.Contains(t, err.Error(), "assigned to 2")
	})

	t.Run("predicate served by no group is not a move", func(t *testing.T) {
		s := &Server{state: &pb.MembershipState{Groups: map[uint32]*pb.Group{}}}
		s.blockCommitsOn = new(sync.Map)
		reason, err := s.checkPreds([]string{"1-" + pred})
		require.Error(t, err)
		require.Equal(t, abortReasonUncategorized, reason,
			"an unserved predicate is not a move; retrying may never succeed")
	})

	t.Run("malformed predicate key is not a move", func(t *testing.T) {
		s := servingIn(1)
		for _, pkey := range []string{pred, "x-" + pred} {
			reason, err := s.checkPreds([]string{pkey})
			require.Error(t, err, "pkey %q must abort", pkey)
			require.Equal(t, abortReasonUncategorized, reason,
				"a malformed predicate key %q is not a move and can never succeed on retry", pkey)
		}
	})

	t.Run("healthy predicate does not abort", func(t *testing.T) {
		reason, err := servingIn(1).checkPreds([]string{"1-" + pred})
		require.NoError(t, err)
		require.Equal(t, abortReasonUncategorized, reason)
	})
}

// TestHasConflictStaleStartTs pins the exact discriminator commit() uses to choose the
// stale-startts reason: a transaction whose startTs is below the leader's startTxnTs floor is a
// conflict, and is flagged stale; a fresh startTs with no conflicting keys is neither.
func TestHasConflictStaleStartTs(t *testing.T) {
	o := &Oracle{}
	o.Init()
	defer o.close()

	o.updateStartTxnTs(100)

	// startTs below the floor: hasConflict true, and the stale discriminator true.
	stale := &api.TxnContext{StartTs: 42}
	require.True(t, o.hasConflict(stale), "transaction below startTxnTs must conflict")
	require.True(t, o.isStaleStartTs(stale), "must be flagged stale")
	require.Equal(t, conflictAbortReason(true), conflictAbortReason(o.isStaleStartTs(stale)))

	// startTs at/above the floor with no keys: not a conflict, not stale.
	fresh := &api.TxnContext{StartTs: 100}
	require.False(t, o.hasConflict(fresh), "fresh transaction with no keys must not conflict")
	require.False(t, o.isStaleStartTs(fresh), "must not be flagged stale")
}

// TestLateAbortStaleStartTs guards the late-abort path. Oracle.commit re-runs hasConflict and
// collapses a stale start timestamp and a genuine keyCommit conflict into the same x.ErrConflict, so
// before this fix every late abort was reported as "conflict" and the stale-startts category was
// reachable only from the early check. Both causes are exercised here through the same discriminator
// commit() now uses.
func TestLateAbortStaleStartTs(t *testing.T) {
	o := &Oracle{}
	o.Init()
	defer o.close()
	o.updateStartTxnTs(100)

	// Stale: Oracle.commit rejects it, and the late path must report stale-startts, not conflict.
	stale := &api.TxnContext{StartTs: 42, CommitTs: 200}
	require.Error(t, o.commit(stale), "a stale start timestamp must be rejected by Oracle.commit")
	require.True(t, o.isStaleStartTs(stale))
	require.True(t, strings.HasPrefix(conflictAbortReason(o.isStaleStartTs(stale)),
		abortReasonStaleStartTs+": "), "late stale abort must not be labelled a conflict")

	// A genuine write-write conflict above the floor still reports conflict.
	first := &api.TxnContext{StartTs: 100, CommitTs: 150, Keys: []string{"a"}}
	require.NoError(t, o.commit(first))
	second := &api.TxnContext{StartTs: 120, CommitTs: 200, Keys: []string{"a"}}
	require.Error(t, o.commit(second), "second writer of key a must conflict")
	require.False(t, o.isStaleStartTs(second), "above the floor, so not stale")
	require.True(t, strings.HasPrefix(conflictAbortReason(o.isStaleStartTs(second)),
		abortReasonConflict+": "), "a real write-write conflict must stay a conflict")
}
