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

// TestValueLockedPrefixes covers locking dynamically-named predicates by prefix.
// A namespace whose predicates are created one per (namespace, relation) at
// runtime cannot enumerate them in ValueLocked, so without prefix locking they
// would be creatable via Alter yet writable by anyone through /mutate.
func TestValueLockedPrefixes(t *testing.T) {
	RegisterReservedNamespace(ReservedNamespace{
		PredicatePrefix:     "dgraph.prefixlock.rel.",
		Predicates:          []string{"dgraph.prefixlock.xid"},
		ValueLockedPrefixes: []string{"dgraph.prefixlock.rel."},
		TrustMarker:         testTrust,
	})

	// Any predicate under the locked prefix, including ones that do not exist
	// yet — that is the point of locking by prefix.
	for _, p := range []string{
		"dgraph.prefixlock.rel.document.owner",
		"dgraph.prefixlock.rel.group.member",
		"dgraph.prefixlock.rel.",
		"dgraph.prefixlock.REL.Document.Owner", // case-insensitive, like exact names
	} {
		marker, locked := ReservedPredicateValueLock(p)
		require.Truef(t, locked, "prefix lock must cover %q", p)
		require.Equal(t, testTrust, marker)
	}

	// Owned but deliberately not locked: xid stays writable so admin tooling and
	// migrations can create nodes.
	_, locked := ReservedPredicateValueLock("dgraph.prefixlock.xid")
	require.False(t, locked)

	// A near miss outside the prefix is not locked.
	_, locked = ReservedPredicateValueLock("dgraph.prefixlock.relative")
	require.False(t, locked)
}

// TestValueLockedExactWinsOverPrefix pins the precedence: a namespace may lock a
// whole prefix to one marker while pinning an individual predicate under it to
// another, so the exact entry must be consulted first.
// TestValueLockedExactWinsOverPrefix pins that an exact value lock takes precedence
// over a prefix that also matches.
//
// It registers the prefix and the exact name in SEPARATE namespaces with distinct
// markers, which is both the only way the precedence is observable and the only way
// the split is expressible: a ReservedNamespace carries one TrustMarker, so the
// earlier version of this test registered both kinds in one namespace, gave them the
// same marker, and passed whichever table ReservedPredicateValueLock consulted
// first.
func TestValueLockedExactWinsOverPrefix(t *testing.T) {
	type prefixTrustKey int
	type exactTrustKey int
	const (
		prefixTrust prefixTrustKey = 1
		exactTrust  exactTrustKey  = 2
	)

	// The namespace that owns the whole sub-namespace by prefix.
	RegisterReservedNamespace(ReservedNamespace{
		PredicatePrefix:     "dgraph.precedence.rel.",
		ValueLockedPrefixes: []string{"dgraph.precedence.rel."},
		TrustMarker:         prefixTrust,
	})
	// A second namespace pinning one predicate inside it to its own marker. Note
	// there is no cross-kind conflict check, so this registration is accepted — the
	// precedence rule is what decides the overlap.
	RegisterReservedNamespace(ReservedNamespace{
		Predicates:  []string{"dgraph.precedence.rel.special"},
		ValueLocked: []string{"dgraph.precedence.rel.special"},
		TrustMarker: exactTrust,
	})

	marker, locked := ReservedPredicateValueLock("dgraph.precedence.rel.special")
	require.True(t, locked)
	require.Equal(t, exactTrust, marker,
		"the exact entry must win over the prefix that also matches")

	marker, locked = ReservedPredicateValueLock("dgraph.precedence.rel.ordinary")
	require.True(t, locked)
	require.Equal(t, prefixTrust, marker,
		"a predicate with no exact entry falls to the prefix owner")
}

// TestValueLockedPrefixRejectsEmpty covers the guard that PredicatePrefix has and
// value locks did not. An empty prefix matches every predicate in the cluster.
func TestValueLockedPrefixRejectsEmpty(t *testing.T) {
	type emptyTrustKey int
	require.PanicsWithValue(t,
		"x.RegisterReservedNamespace: value-locked prefix must not be empty",
		func() {
			RegisterReservedNamespace(ReservedNamespace{
				PredicatePrefix:     "dgraph.emptylock.",
				ValueLockedPrefixes: []string{""},
				TrustMarker:         emptyTrustKey(1),
			})
		})
}

// TestValueLockedPrefixesRequireTrustMarker mirrors the ValueLocked invariant:
// a locked prefix with no marker would be unwritable by everyone, including its
// owner, so it must panic at registration.
func TestValueLockedPrefixesRequireTrustMarker(t *testing.T) {
	require.Panics(t, func() {
		RegisterReservedNamespace(ReservedNamespace{
			PredicatePrefix:     "dgraph.prefixnomarker.rel.",
			ValueLockedPrefixes: []string{"dgraph.prefixnomarker.rel."},
			// TrustMarker intentionally left nil.
		})
	})
}

// TestValueLockedPrefixesRejectQualified confirms a namespace-qualified prefix is
// rejected, for the same reason a qualified exact name is: the guard matches the
// bare predicate, so it would never fire and the predicates would stay writable.
func TestValueLockedPrefixesRejectQualified(t *testing.T) {
	require.Panics(t, func() {
		RegisterReservedNamespace(ReservedNamespace{
			ValueLockedPrefixes: []string{NamespaceAttr(RootNamespace, "dgraph.qualprefix.rel.")},
			TrustMarker:         testTrust,
		})
	})
}

// TestValueLockedPrefixesRejectDuplicate confirms two namespaces cannot claim the
// same locked prefix, since import order would silently pick the TrustMarker.
func TestValueLockedPrefixesRejectDuplicate(t *testing.T) {
	require.Panics(t, func() {
		RegisterReservedNamespace(ReservedNamespace{
			ValueLockedPrefixes: []string{"dgraph.dupprefix.rel."},
			TrustMarker:         testTrust,
		})
		RegisterReservedNamespace(ReservedNamespace{
			ValueLockedPrefixes: []string{"dgraph.dupprefix.rel."},
			TrustMarker:         testTrust,
		})
	})
}
