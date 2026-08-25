/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUidMembershipFilter(t *testing.T) {
	f := uidMembershipFilter([]uint64{2, 5, 9})

	// In-set uids are accepted; out-of-set rejected. Query/result vectors are
	// irrelevant to a membership filter, so nil is fine.
	require.True(t, f(nil, nil, 2))
	require.True(t, f(nil, nil, 5))
	require.True(t, f(nil, nil, 9))
	require.False(t, f(nil, nil, 1))
	require.False(t, f(nil, nil, 0))
	require.False(t, f(nil, nil, 100))
}

func TestUidMembershipFilter_Empty(t *testing.T) {
	// An empty allow-set rejects everything (nothing is in scope).
	f := uidMembershipFilter(nil)
	require.False(t, f(nil, nil, 1))
	require.False(t, f(nil, nil, 0))
}

func TestUidMembershipFilter_Duplicates(t *testing.T) {
	// Duplicate uids in the input collapse to a single membership entry.
	f := uidMembershipFilter([]uint64{7, 7, 7})
	require.True(t, f(nil, nil, 7))
	require.False(t, f(nil, nil, 8))
}

// uidMembershipFilter documents that it sorts a copy because the input may arrive
// unsorted (and may alias a shared var list other goroutines read). The other tests
// only feed already-sorted slices, so neither property is actually exercised. These
// two close that gap.

func TestUidMembershipFilter_UnsortedInput(t *testing.T) {
	// Membership must be correct even when the allow-set arrives unsorted with dups.
	f := uidMembershipFilter([]uint64{9, 2, 5, 2, 100})
	for _, in := range []uint64{2, 5, 9, 100} {
		require.True(t, f(nil, nil, in), "expected %d in set", in)
	}
	for _, out := range []uint64{0, 1, 3, 6, 99, 101} {
		require.False(t, f(nil, nil, out), "expected %d not in set", out)
	}
}

func TestUidMembershipFilter_DoesNotMutateInput(t *testing.T) {
	// The filter must not sort/reorder its input in place: the caller's slice may be
	// a shared, concurrently-read var list. Build the filter from an unsorted slice
	// and assert the original ordering is preserved.
	shared := []uint64{9, 2, 5, 2, 100}
	before := append([]uint64(nil), shared...)

	f := uidMembershipFilter(shared)
	require.True(t, f(nil, nil, 5)) // force use of the sorted copy

	require.Equal(t, before, shared, "input slice must not be mutated")
}

func TestUidMembershipFilter_ConcurrentInputReadsRaceFree(t *testing.T) {
	// Building filters from a shared slice while other goroutines read it must be
	// race-free (meaningful under `go test -race`): the implementation copies before
	// sorting precisely so it never writes through the shared backing array.
	shared := []uint64{1, 2, 3, 4, 5, 6, 7, 8}
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f := uidMembershipFilter(shared)
			_ = f(nil, nil, 4)
			var sum uint64
			for _, u := range shared { // concurrent read of the shared input
				sum += u
			}
			_ = sum
		}()
	}
	wg.Wait()
}
