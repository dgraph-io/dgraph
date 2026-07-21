/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type testTrustKey int

const testTrust testTrustKey = iota

// ns namespace-attributes a bare name, the way the alter validator and
// no-schema mutation guard see predicate/type names at the call sites.
func ns(name string) string { return NamespaceAttr(RootNamespace, name) }

func TestReservedNamespaceRegistry(t *testing.T) {
	RegisterReservedNamespace(ReservedNamespace{
		PredicatePrefix: "dgraph.testns.rel.",
		Predicates:      []string{"dgraph.testns.xid", "dgraph.testns.cfg"},
		Types:           []string{"dgraph.testns.node"},
		ValueLocked:     []string{"dgraph.testns.cfg"},
		TrustMarker:     testTrust,
	})

	// Dynamic prefix members.
	require.True(t, IsRegisteredReservedPredicate(ns("dgraph.testns.rel.owner")))
	// Exact predicate members.
	require.True(t, IsRegisteredReservedPredicate(ns("dgraph.testns.xid")))
	require.True(t, IsRegisteredReservedPredicate(ns("dgraph.testns.cfg")))
	// Not owned: a sibling under the same prefix root, a different reserved
	// namespace, and an ordinary predicate.
	require.False(t, IsRegisteredReservedPredicate(ns("dgraph.testns.other")))
	require.False(t, IsRegisteredReservedPredicate(ns("dgraph.graphql.schema")))
	require.False(t, IsRegisteredReservedPredicate(ns("person.name")))

	// Types are tracked separately from predicates.
	require.True(t, IsRegisteredReservedType(ns("dgraph.testns.node")))
	require.False(t, IsRegisteredReservedType(ns("dgraph.testns.xid"))) // a predicate, not a type
	require.False(t, IsRegisteredReservedType(ns("dgraph.graphql")))

	// Value lock matches the bare predicate, like IsOtherReservedPredicate.
	// cfg is locked to the registered marker; xid is owned but not locked.
	marker, locked := ReservedPredicateValueLock("dgraph.testns.cfg")
	require.True(t, locked)
	require.Equal(t, testTrust, marker)
	_, locked = ReservedPredicateValueLock("dgraph.testns.xid")
	require.False(t, locked)
}

// TestReservedNamespaceRejectsUnregistered confirms names no namespace claims
// are never members, so a stock build with no registration keeps the pristine
// reserved-namespace behavior (only pre-defined names exist under `dgraph.`).
func TestReservedNamespaceRejectsUnregistered(t *testing.T) {
	require.False(t, IsRegisteredReservedPredicate(ns("dgraph.unregistered.pred")))
	require.False(t, IsRegisteredReservedType(ns("dgraph.unregistered.type")))
	_, locked := ReservedPredicateValueLock("dgraph.unregistered.pred")
	require.False(t, locked)
}

// TestReservedPredicateValueLockCaseInsensitive guards against bypassing a value
// lock by changing the case of an owned name: ownership is matched
// case-insensitively, so the value lock must be too.
func TestReservedPredicateValueLockCaseInsensitive(t *testing.T) {
	RegisterReservedNamespace(ReservedNamespace{
		Predicates:  []string{"dgraph.casetest.Secret"},
		ValueLocked: []string{"dgraph.casetest.Secret"},
		TrustMarker: testTrust,
	})

	for _, p := range []string{"dgraph.casetest.Secret", "dgraph.casetest.secret", "dgraph.casetest.SECRET"} {
		marker, locked := ReservedPredicateValueLock(p)
		require.Truef(t, locked, "value lock must hold regardless of case: %q", p)
		require.Equal(t, testTrust, marker)
	}
}

// TestRegisterReservedNamespaceRequiresTrustMarker confirms the invariant is
// enforced at registration (init time): ValueLocked without a TrustMarker would
// make the predicate unwritable by everyone, so it panics rather than failing
// silently at mutation time.
func TestRegisterReservedNamespaceRequiresTrustMarker(t *testing.T) {
	require.Panics(t, func() {
		RegisterReservedNamespace(ReservedNamespace{
			Predicates:  []string{"dgraph.nomarker.cfg"},
			ValueLocked: []string{"dgraph.nomarker.cfg"},
			// TrustMarker intentionally left nil.
		})
	})
}

// TestRegisterReservedNamespaceRejectsQualifiedName confirms a namespace-qualified
// name is rejected at registration. The value-lock guard matches the bare
// predicate, so a qualified entry would never match and the predicate would stay
// publicly writable — it must fail closed at startup instead.
func TestRegisterReservedNamespaceRejectsQualifiedName(t *testing.T) {
	require.Panics(t, func() {
		RegisterReservedNamespace(ReservedNamespace{
			Predicates:  []string{"dgraph.qualtest.secret"},
			ValueLocked: []string{NamespaceAttr(RootNamespace, "dgraph.qualtest.secret")},
			TrustMarker: testTrust,
		})
	})
}

// TestRegisterReservedNamespaceRejectsDuplicate confirms a name claimed twice
// panics rather than silently overwriting (for value locks, last-writer-wins
// would let import order pick the TrustMarker).
func TestRegisterReservedNamespaceRejectsDuplicate(t *testing.T) {
	require.Panics(t, func() {
		RegisterReservedNamespace(ReservedNamespace{Predicates: []string{"dgraph.duptest.x"}})
		RegisterReservedNamespace(ReservedNamespace{Predicates: []string{"dgraph.duptest.x"}})
	})
}
