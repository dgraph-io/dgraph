/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
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
