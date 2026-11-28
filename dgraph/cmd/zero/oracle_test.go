/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package zero

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/dgraph-io/ristretto/v2/z"
)

func TestPurgeBelowDoesNotLowerStartTxnTs(t *testing.T) {
	// Only the state that commit, purgeBelow and updateStartTxnTs use; Init would also start
	// the delta sender goroutine.
	o := &Oracle{commits: make(map[uint64]uint64), keyCommit: z.NewTree("oracle_test")}
	defer o.close()

	key := strconv.FormatUint(42, 36)
	commit := func(startTs, commitTs uint64) error {
		return o.commit(&api.TxnContext{StartTs: startTs, CommitTs: commitTs, Keys: []string{key}})
	}

	// T2 starts at 61, writes key and commits at 70. T1 started at 60 and also writes key.
	require.NoError(t, commit(61, 70))

	// This Zero becomes leader (an election, or a restart). updateLeases sets startTxnTs above
	// every ts handed out so far and resets the conflict map, which forgets T2's commit.
	o.updateStartTxnTs(101)

	// The first snapshot after that carries a checkpoint below T1's start ts. Purging must not
	// lower startTxnTs: the conflict map no longer knows about T2, so T1 must still abort.
	// Only the abort is checked, not which error reports it.
	o.purgeBelow(50)
	require.Error(t, commit(60, 110), "a transaction that started before the leader change must abort")

	// A purge with a higher checkpoint still raises startTxnTs.
	o.purgeBelow(150)
	require.Error(t, commit(120, 160))
	require.NoError(t, commit(155, 170))
}
