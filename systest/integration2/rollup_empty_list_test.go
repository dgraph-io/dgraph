//go:build integration2

/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/dgraph-io/dgraph/v25/dgraphapi"
	"github.com/dgraph-io/dgraph/v25/dgraphtest"
)

// Once incremental rollup has rewritten a list that a delete emptied, the empty marker must not be
// served to transactions that started before the delete, and must carry its real version in an
// incremental backup.
//
// Rollup skips a key it rolled up less than 10s ago, so the tests wait emptyRollupCooldown between
// the setup mutation and the delete, then give the delete's rollup emptyRollupWait to land.

const (
	emptyRollupCooldown = 11 * time.Second
	emptyRollupWait     = 20 * time.Second
	emptyRollupNodes    = 10
)

func startEmptyRollupCluster(t *testing.T) (*dgraphtest.LocalCluster, *dgraphapi.GrpcClient,
	*dgraphapi.HTTPClient) {
	t.Helper()
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1)
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	t.Cleanup(func() { c.Cleanup(t.Failed()) })
	require.NoError(t, c.Start())

	gc, cleanup, err := c.Client()
	require.NoError(t, err)
	t.Cleanup(cleanup)
	hc, err := c.HTTPClient()
	require.NoError(t, err)

	require.NoError(t, gc.SetupSchema(`
		friend: [uid] .
		status: string .`))
	return c, gc, hc
}

// addNodesWithOneFriend creates n nodes with one friend edge and status "before" each.
func addNodesWithOneFriend(t *testing.T, gc *dgraphapi.GrpcClient, n int) []string {
	t.Helper()
	var set strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&set, "_:x%d <friend> _:y%d .\n_:x%d <status> \"before\" .\n", i, i, i)
	}
	resp, err := gc.Mutate(&api.Mutation{SetNquads: []byte(set.String()), CommitNow: true})
	require.NoError(t, err)
	uids := make([]string, n)
	for i := range uids {
		uids[i] = resp.Uids[fmt.Sprintf("x%d", i)]
		require.NotEmpty(t, uids[i])
	}
	return uids
}

// deleteAllFriends empties every node's friend list and returns the commit ts.
func deleteAllFriends(t *testing.T, gc *dgraphapi.GrpcClient, uids []string) uint64 {
	t.Helper()
	var del, set strings.Builder
	for _, uid := range uids {
		fmt.Fprintf(&del, "<%s> <friend> * .\n", uid)
		fmt.Fprintf(&set, "<%s> <status> \"after\" .\n", uid)
	}
	resp, err := gc.Mutate(&api.Mutation{
		DelNquads: []byte(del.String()),
		SetNquads: []byte(set.String()),
		CommitNow: true,
	})
	require.NoError(t, err)
	require.NotZero(t, resp.Txn.CommitTs)
	return resp.Txn.CommitTs
}

func countNodesWithFriend(t *testing.T, gc *dgraphapi.GrpcClient) int {
	t.Helper()
	resp, err := gc.Query(`{ q(func: has(friend)) { count(uid) } }`)
	require.NoError(t, err)
	var out struct {
		Q []struct{ Count int } `json:"q"`
	}
	require.NoError(t, json.Unmarshal(resp.Json, &out))
	require.Len(t, out.Q, 1)
	return out.Q[0].Count
}

// A transaction that started before a delete must keep seeing the deleted edge at its own start
// ts, also after the alpha has rolled the emptied list up.
func TestReadAtEarlierStartTsAfterDeleteEmptiesList(t *testing.T) {
	_, gc, _ := startEmptyRollupCluster(t)
	uids := addNodesWithOneFriend(t, gc, emptyRollupNodes)
	require.Equal(t, emptyRollupNodes, countNodesWithFriend(t, gc))

	time.Sleep(emptyRollupCooldown)

	// Transaction A: its first query fixes its start ts.
	ctx := context.Background()
	txnA := gc.NewReadOnlyTxn()
	defer func() { _ = txnA.Discard(ctx) }()
	resp, err := txnA.Query(ctx, `{ q(func: uid(0x1)) { uid } }`)
	require.NoError(t, err)
	startTs := resp.Txn.StartTs
	require.NotZero(t, startTs)

	commitTs := deleteAllFriends(t, gc, uids)
	require.Greater(t, commitTs, startTs)
	require.Equal(t, 0, countNodesWithFriend(t, gc))

	// A must see every friend at its start ts, now and after the rollup rewrites the keys.
	uidList := strings.Join(uids, ", ")
	query := fmt.Sprintf(`{
		q(func: uid(%s)) { uid status cnt: count(friend) friend { uid } }
		h(func: uid(%s)) @filter(has(friend)) { uid }
	}`, uidList, uidList)
	deadline := time.Now().Add(emptyRollupWait)
	for round := 0; time.Now().Before(deadline); round++ {
		resp, err := txnA.Query(ctx, query)
		require.NoError(t, err)
		require.Equal(t, startTs, resp.Txn.StartTs)

		var out struct {
			Q []struct {
				Uid    string `json:"uid"`
				Status string `json:"status"`
				Cnt    int    `json:"cnt"`
				Friend []struct {
					Uid string `json:"uid"`
				} `json:"friend"`
			} `json:"q"`
			H []struct {
				Uid string `json:"uid"`
			} `json:"h"`
		}
		require.NoError(t, json.Unmarshal(resp.Json, &out))
		require.Len(t, out.Q, emptyRollupNodes)

		missing := 0
		for _, n := range out.Q {
			require.Equal(t, "before", n.Status,
				"round %d: scalar read at start ts %d (delete committed at %d)", round, startTs, commitTs)
			if len(n.Friend) != 1 || n.Cnt != 1 {
				missing++
			}
		}
		require.Zero(t, missing, "round %d: %d of %d nodes lost their friend at start ts %d, "+
			"which precedes the delete committed at %d; response: %s",
			round, missing, emptyRollupNodes, startTs, commitTs, resp.Json)
		require.Len(t, out.H, emptyRollupNodes,
			"round %d: has(friend) at start ts %d dropped nodes after the rollup", round, startTs)

		time.Sleep(500 * time.Millisecond)
	}
}

// A full backup taken while the edges exist plus an incremental backup taken after the delete and
// its rollup must restore to zero edges.
func TestRestoreAfterDeleteEmptiesListAndRollup(t *testing.T) {
	c, gc, hc := startEmptyRollupCluster(t)
	uids := addNodesWithOneFriend(t, gc, emptyRollupNodes)
	require.Equal(t, emptyRollupNodes, countNodesWithFriend(t, gc))

	require.NoError(t, hc.Backup(c, true, dgraphtest.DefaultBackupDir))

	time.Sleep(emptyRollupCooldown)
	deleteAllFriends(t, gc, uids)
	require.Equal(t, 0, countNodesWithFriend(t, gc))

	// Give the rollup time to rewrite the emptied keys.
	deadline := time.Now().Add(emptyRollupWait / 2)
	for time.Now().Before(deadline) {
		require.Equal(t, 0, countNodesWithFriend(t, gc))
		time.Sleep(500 * time.Millisecond)
	}

	require.NoError(t, hc.Backup(c, false, dgraphtest.DefaultBackupDir))

	// An edge in neither backup; a correct restore drops it.
	_, err := gc.Mutate(&api.Mutation{SetNquads: []byte(`_:s <friend> _:t .`), CommitNow: true})
	require.NoError(t, err)
	require.Equal(t, 1, countNodesWithFriend(t, gc))

	require.NoError(t, hc.Restore(c, dgraphtest.DefaultBackupDir, "", 0, 0))
	require.NoError(t, dgraphapi.WaitForRestore(c))

	require.Equal(t, 0, countNodesWithFriend(t, gc),
		"friend edges deleted and rolled up before the incremental backup came back after restore")
}
