/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package bulk

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadShardDirsNumericOrder(t *testing.T) {
	base := t.TempDir()
	// Reduce shard names are not zero padded; a lexical sort would give
	// shard_0, shard_1, shard_10, shard_11, shard_2, ...
	for i := range 12 {
		require.NoError(t, os.MkdirAll(filepath.Join(base, fmt.Sprintf("shard_%d", i)), 0750))
	}
	dirs := readShardDirs(base)
	require.Len(t, dirs, 12)
	for i, dir := range dirs {
		require.Equal(t, fmt.Sprintf("shard_%d", i), filepath.Base(dir),
			"dirs[%d] must be shard_%d so the reducer writes it to out/%d/p (group %d)",
			i, i, i, i+1)
	}
}

func TestShardDirIndex(t *testing.T) {
	for in, want := range map[string]int{"000": 0, "007": 7, "012": 12,
		"shard_0": 0, "shard_11": 11} {
		got, err := shardDirIndex(filepath.Join("tmp", "x", in))
		require.NoError(t, err)
		require.Equal(t, want, got)
	}
	_, err := shardDirIndex("tmp/x/not-a-shard")
	require.Error(t, err)
}
