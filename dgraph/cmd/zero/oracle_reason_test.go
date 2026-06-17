/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package zero

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v250/protos/api"
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

func TestConflictAbortReason(t *testing.T) {
	// Write-write conflict.
	r := conflictAbortReason(false)
	require.True(t, strings.HasPrefix(r, abortReasonConflict+": "),
		"want conflict prefix, got %q", r)
	require.Equal(t, abortReason(abortReasonConflict, abortDetailConflict), r)

	// Stale start timestamp (leader change).
	r = conflictAbortReason(true)
	require.True(t, strings.HasPrefix(r, abortReasonStaleStartTs+": "),
		"want stale-startts prefix, got %q", r)
	require.Equal(t, abortReason(abortReasonStaleStartTs, abortDetailStaleStartTs), r)
	require.Contains(t, r, "leader change")
}

// TestHasConflictStaleStartTs pins the exact discriminator commit() uses to choose the
// stale-startts reason: a txn whose startTs predates the leader's startTxnTs lease is a
// conflict, and is flagged stale; a fresh startTs with no conflicting keys is neither.
func TestHasConflictStaleStartTs(t *testing.T) {
	o := &Oracle{}
	o.Init()
	defer o.close()

	o.updateStartTxnTs(100)

	// startTs below the lease floor: hasConflict true, and the stale discriminator true.
	stale := &api.TxnContext{StartTs: 42}
	require.True(t, o.hasConflict(stale), "txn below startTxnTs must conflict")
	require.True(t, stale.StartTs < o.startTxnTs, "must be flagged stale")
	require.Equal(t, conflictAbortReason(true), conflictAbortReason(stale.StartTs < o.startTxnTs))

	// startTs at/above the lease floor with no keys: not a conflict, not stale.
	fresh := &api.TxnContext{StartTs: 100}
	require.False(t, o.hasConflict(fresh), "fresh txn with no keys must not conflict")
	require.False(t, fresh.StartTs < o.startTxnTs, "must not be flagged stale")
}
