/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package edgraph

import (
	"context"
	"testing"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/dgraph-io/dgraph/v25/x"
	"github.com/stretchr/testify/require"
)

type guardTrustKey struct{}

// guardTestNamespace registers a synthetic reserved namespace covering both lock
// styles: one exact predicate and one dynamic prefix. Registration is global and
// panics on a duplicate, so it runs once.
func init() {
	x.RegisterReservedNamespace(x.ReservedNamespace{
		PredicatePrefix:     "dgraph.guardtest.rel.",
		Predicates:          []string{"dgraph.guardtest.xid", "dgraph.guardtest.cfg"},
		ValueLocked:         []string{"dgraph.guardtest.cfg"},
		ValueLockedPrefixes: []string{"dgraph.guardtest.rel."},
		TrustMarker:         guardTrustKey{},
	})
}

// TestReservedPredicateGuardValueLocks covers the mutation-path guard for both
// exact-name and prefix value locks. The prefix case is what protects predicates
// created dynamically at runtime, which cannot be enumerated up front — for
// an authorization store that is every stored grant, so an unguarded prefix means they
// are forgeable through plain /mutate.
func TestReservedPredicateGuardValueLocks(t *testing.T) {
	nq := func(pred string) *api.NQuad {
		return &api.NQuad{Subject: "0x1", Predicate: pred}
	}

	t.Run("untrusted context is denied", func(t *testing.T) {
		guard := newReservedPredicateGuard(context.Background())

		for _, pred := range []string{
			"dgraph.guardtest.cfg",                // exact lock
			"dgraph.guardtest.rel.document.owner", // prefix lock
			"dgraph.guardtest.rel.never_seen.yet", // prefix lock, predicate not yet created
			"dgraph.guardtest.REL.Document.Owner", // case-insensitive
		} {
			require.Errorf(t, guard(nq(pred)), "value-locked predicate %q must be denied", pred)
		}
	})

	t.Run("trusted context is allowed", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), guardTrustKey{}, true)
		guard := newReservedPredicateGuard(ctx)

		// This is the case an owning service's write path depends on: its in-process
		// client sets the marker on the context it builds, so
		// locking the relation prefix must not break WriteRelationships.
		for _, pred := range []string{
			"dgraph.guardtest.cfg",
			"dgraph.guardtest.rel.document.owner",
			"dgraph.guardtest.rel.never_seen.yet",
		} {
			require.NoErrorf(t, guard(nq(pred)), "trusted writer must be allowed to write %q", pred)
		}
	})

	t.Run("a false marker is not trusted", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), guardTrustKey{}, false)
		guard := newReservedPredicateGuard(ctx)
		require.Error(t, guard(nq("dgraph.guardtest.rel.document.owner")))
	})

	t.Run("unlocked and unowned predicates pass", func(t *testing.T) {
		guard := newReservedPredicateGuard(context.Background())

		for _, pred := range []string{
			"dgraph.guardtest.xid",       // owned but deliberately not locked
			"dgraph.guardtest.relx.evil", // near miss outside the locked prefix
			"person.name",
		} {
			require.NoErrorf(t, guard(nq(pred)), "predicate %q must not be value-locked", pred)
		}
	})
}
