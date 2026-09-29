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

// writeMapShard fabricates tmp/map_output/<name> holding a single file of the given size.
func writeMapShard(t *testing.T, tmpDir, name string, size int) {
	t.Helper()
	dir := filepath.Join(tmpDir, mapShardDir, name)
	require.NoError(t, os.MkdirAll(dir, 0750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "chunk.map.gz"), make([]byte, size), 0600))
}

// reduceShardContents returns the map shard names inside each tmp/shards/shard_<i>.
func reduceShardContents(t *testing.T, tmpDir string, reduceShards int) [][]string {
	t.Helper()
	out := make([][]string, reduceShards)
	for i := range reduceShards {
		dir := filepath.Join(tmpDir, reduceShardDir, fmt.Sprintf("shard_%d", i))
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		for _, e := range entries {
			out[i] = append(out[i], e.Name())
		}
	}
	return out
}

func TestMergeMapShardsLegacyPacking(t *testing.T) {
	tmpDir := t.TempDir()
	writeMapShard(t, tmpDir, "000", 10)
	writeMapShard(t, tmpDir, "001", 1000)
	writeMapShard(t, tmpDir, "002", 100)

	opt := &BulkOptions{TmpDir: tmpDir, ReduceShards: 2}
	mergeMapShardsIntoReduceShards(opt)

	// 000 carries reserved predicates -> shard_0 unconditionally; then the largest map shard
	// goes to the emptiest reduce shard: 001 -> shard_1, 002 -> shard_0.
	require.Equal(t, [][]string{{"000", "002"}, {"001"}},
		reduceShardContents(t, tmpDir, 2))
}

func TestMergeMapShardsWithPlacement(t *testing.T) {
	tmpDir := t.TempDir()
	writeMapShard(t, tmpDir, "000", 10)
	writeMapShard(t, tmpDir, "001", 20)
	writeMapShard(t, tmpDir, "002", 500)
	writeMapShard(t, tmpDir, "003", 50)

	opt := &BulkOptions{
		TmpDir:          tmpDir,
		ReduceShards:    2,
		tabletPlacement: map[string]int{"0-payload": 1},
	}
	mergeMapShardsIntoReduceShards(opt)

	// Identity for group-designated shards: 000 -> shard_0, 001 -> shard_1. The rest are
	// size-packed: 002 (500B) -> shard_0 (10B, emptiest), then 003 -> shard_1.
	require.Equal(t, [][]string{{"000", "002"}, {"001", "003"}},
		reduceShardContents(t, tmpDir, 2))
}

func TestMergeMapShardsPlacementMissingDesignatedShard(t *testing.T) {
	tmpDir := t.TempDir()
	// Map shard 001 (group 2) never materialized: shard dirs are created lazily and no
	// data was routed there. Shard 005 is an ordinary packable shard.
	writeMapShard(t, tmpDir, "000", 10)
	writeMapShard(t, tmpDir, "002", 100)
	writeMapShard(t, tmpDir, "005", 30)

	opt := &BulkOptions{
		TmpDir:          tmpDir,
		ReduceShards:    3,
		tabletPlacement: map[string]int{"0-a": 0, "0-b": 2},
	}
	mergeMapShardsIntoReduceShards(opt)

	// shard_1 exists but is empty; 005 packs into it (the emptiest).
	require.Equal(t, [][]string{{"000"}, {"005"}, {"002"}},
		reduceShardContents(t, tmpDir, 3))
}

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
