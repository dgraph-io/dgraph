/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package posting

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/tok/index"
	"github.com/dgraph-io/dgraph/v25/x"
)

// TestViLocalCacheGetReturnsErrNotFoundOnMiss pins the contract the query-path
// cache must honor on a genuine miss: it surfaces the shared index.ErrNotFound
// sentinel (so kmeans centroid hydration caches the definitive miss instead of
// re-reading the centroid key under a write lock on every search), while
// errors.Is(_, ErrNoValue) still holds for existing callers.
//
// tok/kmeans' fakeCache already returns index.ErrNotFound on a miss; the real
// viLocalCache is what the query path actually uses (worker/task.go). tok can't
// import posting, so this is where the two are kept in step.
func TestViLocalCacheGetReturnsErrNotFoundOnMiss(t *testing.T) {
	attr := x.AttrInRootNamespace("vihydrationmisspred")
	key := x.DataKey(attr, 1)
	vc := NewViLocalCache(NewLocalCache(1))

	// Get is the accessor the query-path hydration (kmeans maybeHydrate) uses.
	// It shares GetValueFromPostingList with GetWithLockHeld, so pinning it
	// pins the sentinel for both.
	_, err := vc.Get(key)
	require.Error(t, err)
	require.True(t, errors.Is(err, index.ErrNotFound),
		"query-path miss must surface index.ErrNotFound; got %v", err)
	require.True(t, errors.Is(err, ErrNoValue),
		"ErrNoValue back-compat must still hold; got %v", err)
}
