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
