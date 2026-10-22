/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package zero

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/x"
)

// The abort-reason wire format is a contract with gRPC clients (e.g. dgraph4j parses the
// "<code>: " prefix into TxnConflictException.AbortReason). These unit tests pin the
// category prefixes and the logic that selects between them, so the contract can't drift
// silently without an integration cluster.

func TestAbortReasonFormat(t *testing.T) {
	const legacy = "Transaction has been aborted. Please retry"
	require.Equal(t, legacy, abortLegacyMessage, "the legacy sentence must never change")
	require.Equal(t, "conflict: "+legacy+". boom", abortReason(abortReasonConflict, "boom"))
	require.Equal(t, "stale-startts: "+legacy+". x", abortReason(abortReasonStaleStartTs, "x"))
	require.Equal(t, "predicate-move: "+legacy+". y", abortReason(abortReasonPredicateMove, "y"))
	require.Equal(t, legacy+". z", abortReason(abortReasonUncategorized, "z"))
}

// TestAbortVocabulary pins the entire published surface: every category, and every detail a client
// can receive for a server-decided abort, with the category each detail pairs with. Because the
// details are declared in one block in oracle.go, adding a new abort message without deciding its
// category — or quietly rewording an existing one that dgraph4j, pydgraph or the docs depend on —
// fails here. The wording assertions are the pre-abort-reason text verbatim, so hoisting these to
// constants is provably a move and not an edit.
func TestAbortVocabulary(t *testing.T) {
	// The three published codes, plus the withheld sentinel. Clients switch on exactly these.
	require.Equal(t, "", abortReasonUncategorized)
	require.Equal(t, "conflict", abortReasonConflict)
	require.Equal(t, "stale-startts", abortReasonStaleStartTs)
	require.Equal(t, "predicate-move", abortReasonPredicateMove)

	for _, tc := range []struct {
		name     string
		category string
		rendered string
	}{
		{"write-write conflict", abortReasonConflict,
			abortDetailConflict},
		{"stale start timestamp", abortReasonStaleStartTs,
			abortDetailStaleStartTs},
		{"move in flight", abortReasonPredicateMove,
			fmt.Sprintf(abortDetailPredicateBlockedFmt, "friend")},
		{"move completed", abortReasonPredicateMove,
			fmt.Sprintf(abortDetailGroupMismatchFmt, 1, "friend", 2)},
		{"predicate served by no group", abortReasonUncategorized,
			fmt.Sprintf(abortDetailTabletNilFmt, "friend")},
		{"malformed key, no separator", abortReasonUncategorized,
			fmt.Sprintf(abortDetailMissingGroupIDFmt, "1foo")},
		{"malformed key, bad group id", abortReasonUncategorized,
			fmt.Sprintf(abortDetailBadGroupIDFmt, "xfoo")},
		{"aborted out of band before commit", abortReasonUncategorized,
			abortDetailPreAborted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wire := abortReason(tc.category, tc.rendered)
			// Every abort, categorized or not, must still contain the legacy sentence. Clients that
			// spot aborts by message text (dgraph-js-http requires "abort" and "retry") would
			// otherwise treat it as a hard failure instead of retrying.
			lower := strings.ToLower(wire)
			require.Contains(t, lower, "abort")
			require.Contains(t, lower, "retry")
			legacy := abortLegacyMessage + ". " + tc.rendered
			if tc.category == abortReasonUncategorized {
				require.Equal(t, legacy, wire, "a withheld cause must carry no prefix")
				return
			}
			require.Equal(t, tc.category+": "+legacy, wire)
		})
	}

	// The conflict detail names what it cannot narrow — a conflict key is a one-way fingerprint by
	// the time Zero matches it, so data, index, count and @upsert conflicts are indistinguishable.
	// Callers who set one scalar value do not expect the keys derived from it to conflict, so the
	// possibilities are spelled out.
	for _, want := range []string{"index", "count", "@upsert"} {
		require.Contains(t, abortDetailConflict, want,
			"conflict detail should name the key kinds it cannot distinguish between")
	}

	// The out-of-band abort names its known causes rather than guessing a category. None of them is
	// a predicate move, and the abort proposal that delivers each records no cause, so labelling
	// this predicate-move would be wrong. Before it was handled, commit() returned nil and the abort
	// reached the client as a bare dgo.ErrAborted with no reason at all.
	for _, want := range []string{"schema update", "drop-predicate", "txn-abort-after", "discarded"} {
		require.Contains(t, abortDetailPreAborted, want,
			"pre-abort detail should name the causes it cannot distinguish between")
	}

	// Backwards-compatibility guard: the conflict abort on the wire is byte-identical to what it was
	// before abortReason took over the legacy sentence. dgraph/cmd/alpha/upsert_test.go loops while
	// the live server error contains that sentence, and query/query4_test.go and
	// systest/unique_test.go assert Contains on it.
	require.Equal(t, "conflict: Transaction has been aborted. Please retry. Another transaction "+
		"committed to one of the same keys. The conflicting key cannot be identified: it may be the "+
		"data key written directly, or an index or count key derived from it. On an @upsert "+
		"predicate the uid is excluded, so any two transactions writing the same value conflict",
		conflictAbortReason(false))

	// Wording of the checkPreds details, verbatim from before the abort-reason work.
	require.Equal(t, "Unable to find group id in 1foo",
		fmt.Sprintf(abortDetailMissingGroupIDFmt, "1foo"))
	require.Equal(t, "unable to parse group id from xfoo",
		fmt.Sprintf(abortDetailBadGroupIDFmt, "xfoo"))
	require.Equal(t, "Tablet for friend is nil",
		fmt.Sprintf(abortDetailTabletNilFmt, "friend"))
	require.Equal(t, "Commits on predicate friend are blocked due to predicate move",
		fmt.Sprintf(abortDetailPredicateBlockedFmt, "friend"))
	require.Equal(t, "Mutation done in group: 1. Predicate friend assigned to 2",
		fmt.Sprintf(abortDetailGroupMismatchFmt, 1, "friend", 2))
}

// TestAbortReasonUncategorized pins the withholding contract: when the server cannot substantiate
// one of the published categories it emits the bare detail with no prefix, rather than borrowing a
// category that would imply the wrong remedy. It starts with the same sentence a pre-feature server
// sends for every abort, and the proposed dgraph4j, pydgraph and dgo parsers map a message without
// a category prefix to UNKNOWN. The absence of a colon-delimited prefix is the whole contract, so it
// must not regress.
func TestAbortReasonUncategorized(t *testing.T) {
	require.Equal(t, "Transaction has been aborted. Please retry. Tablet for foo is nil",
		abortReason(abortReasonUncategorized, "Tablet for foo is nil"))
	require.Equal(t, "Transaction has been aborted. Please retry. context canceled",
		abortReason(abortReasonUncategorized, "context canceled"))

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

	t.Run("details name the predicate without its namespace", func(t *testing.T) {
		// The Alpha sends "<gid>-<namespaced attr>", so after the group id is split off the predicate
		// is still "<hex ns>-name". Lookups must use that form, but the client-visible detail must
		// show the name the user wrote.
		for _, ns := range []uint64{x.RootNamespace, 0x2a} {
			attr := x.NamespaceAttr(ns, pred)
			s := &Server{state: &pb.MembershipState{Groups: map[uint32]*pb.Group{
				2: {Tablets: map[string]*pb.Tablet{attr: {GroupId: 2, Predicate: attr}}},
			}}}
			s.blockCommitsOn = new(sync.Map)

			_, err := s.checkPreds([]string{"1-" + attr})
			require.EqualError(t, err, fmt.Sprintf(abortDetailGroupMismatchFmt, 1, pred, 2))

			s.blockCommitsOn.Store(attr, struct{}{})
			_, err = s.checkPreds([]string{"2-" + attr})
			require.EqualError(t, err, fmt.Sprintf(abortDetailPredicateBlockedFmt, pred))

			_, err = s.checkPreds([]string{"2-" + x.NamespaceAttr(ns, "other")})
			require.EqualError(t, err, fmt.Sprintf(abortDetailTabletNilFmt, "other"))
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

// TestLateAbortStaleStartTs guards the late-abort path. Oracle.commit must tell a stale start
// timestamp apart from a genuine keyCommit conflict itself, under its own lock, and lateAbortReason
// must report each under its own category. Re-reading startTxnTs after Oracle.commit returned could
// mislabel the abort, since a snapshot purge or lease renewal can move it in between.
func TestLateAbortStaleStartTs(t *testing.T) {
	o := &Oracle{}
	o.Init()
	defer o.close()
	o.updateStartTxnTs(100)

	stale := o.commit(&api.TxnContext{StartTs: 42, CommitTs: 200})
	require.ErrorIs(t, stale, errStaleStartTs)

	first := &api.TxnContext{StartTs: 100, CommitTs: 150, Keys: []string{"a"}}
	require.NoError(t, o.commit(first))
	conflict := o.commit(&api.TxnContext{StartTs: 120, CommitTs: 200, Keys: []string{"a"}})
	require.ErrorIs(t, conflict, x.ErrConflict, "second writer of key a must conflict")

	for _, tc := range []struct {
		name           string
		orcErr, ctxErr error
		want           string
	}{
		{"stale start ts", stale, nil,
			abortReason(abortReasonStaleStartTs, abortDetailStaleStartTs)},
		{"write-write conflict", conflict, nil,
			abortReason(abortReasonConflict, abortDetailConflict)},
		{"cancelled context", nil, context.Canceled,
			abortReason(abortReasonUncategorized, "context canceled")},
		{"timed out context", nil, context.DeadlineExceeded,
			abortReason(abortReasonUncategorized, "context deadline exceeded")},
		// The oracle would have aborted regardless of the context, so its cause wins.
		{"conflict wins over context", conflict, context.Canceled,
			abortReason(abortReasonConflict, abortDetailConflict)},
		{"stale wins over context", stale, context.DeadlineExceeded,
			abortReason(abortReasonStaleStartTs, abortDetailStaleStartTs)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, aborted := lateAbortReason(tc.orcErr, tc.ctxErr)
			require.True(t, aborted)
			require.Equal(t, tc.want, got)
		})
	}

	got, aborted := lateAbortReason(nil, nil)
	require.False(t, aborted, "nothing rejected the commit")
	require.Empty(t, got)
}

// TestDisplayPred pins that stripping the namespace for display can never panic on a hand-crafted
// Preds entry, which is client-supplied. x.ParseAttr would index past the end of a separator-less
// split.
func TestDisplayPred(t *testing.T) {
	for in, want := range map[string]string{
		"0-name":     "name",
		"2a-name":    "name",
		"0-a-b":      "a-b",
		"nodash":     "nodash",
		"0-":         "0-",
		"":           "",
		"0-name@en":  "name@en",
		"-leadingns": "leadingns",
	} {
		require.Equal(t, want, displayPred(in), "displayPred(%q)", in)
	}
}
