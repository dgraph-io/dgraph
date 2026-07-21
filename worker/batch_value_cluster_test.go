//go:build integration

/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/dgraph-io/dgraph/v25/testutil"
)

// TestClusterBatchedValueReads drives the batched value-posting read path end-to-end
// over a running cluster: client -> query parsing -> ProcessTaskOverNetwork -> alpha's
// processTask -> handleValuePostings -> batched badger reads. 700 uids force multiple
// DivideAndRule goroutines and many 10-key GetBatch chunks on the alpha, with holes
// (never-set uids), exact-value deletes and star delete-alls interleaved so that chunk
// slots mix hits, misses and wiped keys. Every uid must come back with exactly its own
// value or exactly no value — any batching misalignment shows up as a value on the
// wrong uid.
func TestClusterBatchedValueReads(t *testing.T) {
	dg, err := testutil.DgraphClient(testutil.GetSockAddr())
	require.NoError(t, err)
	testutil.DropAll(t, dg)
	require.NoError(t, dg.Alter(context.Background(), &api.Operation{Schema: `bvclust: string .`}))

	const n = 700
	hasVal := func(uid uint64) bool { return uid%7 != 0 }        // uid%7==0: never set (hole)
	deleted := func(uid uint64) bool { return uid%7 != 0 && uid%5 == 0 } // set then deleted

	var set strings.Builder
	for uid := uint64(1); uid <= n; uid++ {
		if hasVal(uid) {
			fmt.Fprintf(&set, "<%#x> <bvclust> \"v%d\" .\n", uid, uid)
		}
	}
	setClusterEdge(t, dg, set.String())

	var delExact, delStar strings.Builder
	for uid := uint64(1); uid <= n; uid++ {
		if !deleted(uid) {
			continue
		}
		if uid%2 == 0 {
			fmt.Fprintf(&delStar, "<%#x> <bvclust> * .\n", uid)
		} else {
			fmt.Fprintf(&delExact, "<%#x> <bvclust> \"v%d\" .\n", uid, uid)
		}
	}
	delClusterEdge(t, dg, delExact.String())
	delClusterEdge(t, dg, delStar.String())

	var uidv []string
	for uid := uint64(1); uid <= n; uid++ {
		uidv = append(uidv, fmt.Sprintf("%#x", uid))
	}
	query := fmt.Sprintf(`{ q(func: uid(%s)) { uid bvclust } }`, strings.Join(uidv, ","))
	resp, err := testutil.RetryQuery(dg, query)
	require.NoError(t, err)

	type row struct {
		Uid     string `json:"uid"`
		Bvclust string `json:"bvclust,omitempty"`
	}
	var got struct {
		Q []row `json:"q"`
	}
	require.NoError(t, json.Unmarshal(resp.Json, &got))
	require.Len(t, got.Q, n)

	byUid := make(map[string]row, len(got.Q))
	for _, r := range got.Q {
		byUid[r.Uid] = r
	}
	for uid := uint64(1); uid <= n; uid++ {
		r, ok := byUid[fmt.Sprintf("%#x", uid)]
		require.Truef(t, ok, "uid %d missing from result", uid)
		if hasVal(uid) && !deleted(uid) {
			require.Equalf(t, fmt.Sprintf("v%d", uid), r.Bvclust, "uid %d has the wrong value", uid)
		} else {
			require.Emptyf(t, r.Bvclust, "uid %d should have no value", uid)
		}
	}
}
