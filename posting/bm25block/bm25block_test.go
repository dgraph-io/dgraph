/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package bm25block

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/posting/bm25enc"
)

func TestDirRoundtrip(t *testing.T) {
	dir := &Dir{
		NextID: 5,
		Blocks: []BlockMeta{
			{FirstUID: 100, BlockID: 0, Count: 128, MaxTF: 10},
			{FirstUID: 500, BlockID: 1, Count: 128, MaxTF: 5},
			{FirstUID: 900, BlockID: 2, Count: 64, MaxTF: 20},
		},
	}
	data := EncodeDir(dir)
	got := DecodeDir(data)
	require.Equal(t, dir.NextID, got.NextID)
	require.Equal(t, dir.Blocks, got.Blocks)
}

func TestDirRoundtripEmpty(t *testing.T) {
	require.Nil(t, EncodeDir(nil))
	require.Nil(t, EncodeDir(&Dir{}))

	got := DecodeDir(nil)
	require.Empty(t, got.Blocks)
	got = DecodeDir([]byte{})
	require.Empty(t, got.Blocks)
}

func TestDirRoundtripSingle(t *testing.T) {
	dir := &Dir{
		NextID: 1,
		Blocks: []BlockMeta{{FirstUID: 42, BlockID: 0, Count: 1, MaxTF: 3}},
	}
	got := DecodeDir(EncodeDir(dir))
	require.Equal(t, dir.Blocks, got.Blocks)
}

func TestFindBlock(t *testing.T) {
	dir := &Dir{
		Blocks: []BlockMeta{
			{FirstUID: 100},
			{FirstUID: 500},
			{FirstUID: 900},
		},
	}
	require.Equal(t, 0, dir.FindBlock(50))   // before first block
	require.Equal(t, 0, dir.FindBlock(100))  // exact first
	require.Equal(t, 0, dir.FindBlock(200))  // within first block
	require.Equal(t, 1, dir.FindBlock(500))  // exact second
	require.Equal(t, 1, dir.FindBlock(700))  // within second block
	require.Equal(t, 2, dir.FindBlock(900))  // exact third
	require.Equal(t, 2, dir.FindBlock(9999)) // beyond last block
}

func TestFindBlockEmpty(t *testing.T) {
	dir := &Dir{}
	require.Equal(t, 0, dir.FindBlock(100))
}

func TestAllocBlockID(t *testing.T) {
	dir := &Dir{NextID: 3}
	require.Equal(t, uint32(3), dir.AllocBlockID())
	require.Equal(t, uint32(4), dir.AllocBlockID())
	require.Equal(t, uint32(5), dir.NextID)
}

func TestSplitIntoBlocks(t *testing.T) {
	// Create 300 entries.
	entries := make([]bm25enc.Entry, 300)
	for i := range entries {
		entries[i] = bm25enc.Entry{UID: uint64(i + 1), Value: uint32(i%10 + 1)}
	}
	dir, blockMap := SplitIntoBlocks(entries)

	// Should split into ceil(300/128) = 3 blocks.
	require.Len(t, dir.Blocks, 3)
	require.Len(t, blockMap, 3)

	// First block: 128 entries.
	require.Equal(t, uint32(128), dir.Blocks[0].Count)
	require.Equal(t, uint64(1), dir.Blocks[0].FirstUID)
	require.Len(t, blockMap[dir.Blocks[0].BlockID], 128)

	// Second block: 128 entries.
	require.Equal(t, uint32(128), dir.Blocks[1].Count)
	require.Equal(t, uint64(129), dir.Blocks[1].FirstUID)

	// Third block: 44 entries.
	require.Equal(t, uint32(44), dir.Blocks[2].Count)
	require.Equal(t, uint64(257), dir.Blocks[2].FirstUID)

	// NextID should be 3.
	require.Equal(t, uint32(3), dir.NextID)
}

func TestSplitIntoBlocksEmpty(t *testing.T) {
	dir, blockMap := SplitIntoBlocks(nil)
	require.Empty(t, dir.Blocks)
	require.Nil(t, blockMap)
}

func TestSplitIntoBlocksSmall(t *testing.T) {
	entries := []bm25enc.Entry{{UID: 1, Value: 5}, {UID: 2, Value: 3}}
	dir, blockMap := SplitIntoBlocks(entries)
	require.Len(t, dir.Blocks, 1)
	require.Equal(t, uint32(2), dir.Blocks[0].Count)
	require.Equal(t, uint32(5), dir.Blocks[0].MaxTF)
	require.Equal(t, entries, blockMap[0])
}

func TestUpdateBlockMeta(t *testing.T) {
	dir := &Dir{
		Blocks: []BlockMeta{{FirstUID: 100, BlockID: 0, Count: 3, MaxTF: 5}},
	}
	entries := []bm25enc.Entry{
		{UID: 50, Value: 2},
		{UID: 100, Value: 8},
		{UID: 200, Value: 3},
		{UID: 300, Value: 1},
	}
	dir.UpdateBlockMeta(0, entries)
	require.Equal(t, uint64(50), dir.Blocks[0].FirstUID)
	require.Equal(t, uint32(4), dir.Blocks[0].Count)
	require.Equal(t, uint32(8), dir.Blocks[0].MaxTF)
}

func TestInsertRemoveBlockMeta(t *testing.T) {
	dir := &Dir{
		Blocks: []BlockMeta{
			{FirstUID: 100, BlockID: 0},
			{FirstUID: 500, BlockID: 1},
		},
	}
	dir.InsertBlockMeta(1, BlockMeta{FirstUID: 300, BlockID: 2})
	require.Len(t, dir.Blocks, 3)
	require.Equal(t, uint64(300), dir.Blocks[1].FirstUID)
	require.Equal(t, uint64(500), dir.Blocks[2].FirstUID)

	dir.RemoveBlockMeta(1)
	require.Len(t, dir.Blocks, 2)
	require.Equal(t, uint64(500), dir.Blocks[1].FirstUID)
}

func TestComputeUBPre(t *testing.T) {
	k, b := 1.2, 0.75

	// maxTF=0 -> 0
	require.Equal(t, 0.0, ComputeUBPre(0, k, b))

	// maxTF=1: 1 * 2.2 / (1 + 1.2*0.25) = 2.2 / 1.3
	expected := 2.2 / 1.3
	require.InEpsilon(t, expected, ComputeUBPre(1, k, b), 1e-9)

	// maxTF=10: 10 * 2.2 / (10 + 1.2*0.25) = 22 / 10.3
	expected = 22.0 / 10.3
	require.InEpsilon(t, expected, ComputeUBPre(10, k, b), 1e-9)

	// With b=0: score = tf*(k+1)/(tf+k) — no length normalization.
	expected = 5.0 * 2.2 / (5.0 + 1.2)
	require.InEpsilon(t, expected, ComputeUBPre(5, k, 0), 1e-9)
}

func TestSuffixMaxUBPre(t *testing.T) {
	dir := &Dir{
		Blocks: []BlockMeta{
			{MaxTF: 1},
			{MaxTF: 10},
			{MaxTF: 3},
		},
	}
	k, b := 1.2, 0.75
	suf := SuffixMaxUBPre(dir, k, b)
	require.Len(t, suf, 3)

	ub0 := ComputeUBPre(1, k, b)
	ub1 := ComputeUBPre(10, k, b)
	ub2 := ComputeUBPre(3, k, b)

	require.InEpsilon(t, math.Max(ub0, math.Max(ub1, ub2)), suf[0], 1e-9)
	require.InEpsilon(t, math.Max(ub1, ub2), suf[1], 1e-9)
	require.InEpsilon(t, ub2, suf[2], 1e-9)
}

func TestSuffixMaxUBPreEmpty(t *testing.T) {
	require.Nil(t, SuffixMaxUBPre(&Dir{}, 1.2, 0.75))
}

func TestMergeAllBlocks(t *testing.T) {
	// Simulate overlapping blocks with a tombstone.
	blocks := map[uint32][]bm25enc.Entry{
		0: {{UID: 1, Value: 3}, {UID: 5, Value: 1}},
		1: {{UID: 5, Value: 7}, {UID: 10, Value: 2}},   // UID 5 overrides
		2: {{UID: 15, Value: 0}, {UID: 20, Value: 4}},   // UID 15 is tombstone
	}
	dir := &Dir{
		Blocks: []BlockMeta{
			{FirstUID: 1, BlockID: 0, Count: 2},
			{FirstUID: 5, BlockID: 1, Count: 2},
			{FirstUID: 15, BlockID: 2, Count: 2},
		},
		NextID: 3,
	}
	newDir, newBlocks := MergeAllBlocks(dir, func(id uint32) []bm25enc.Entry {
		return blocks[id]
	})
	// After merge: UID 1(3), 5(7), 10(2), 20(4) — UID 15 removed (tombstone).
	require.Len(t, newDir.Blocks, 1) // 4 entries fits in one block
	require.Len(t, newBlocks, 1)
	entries := newBlocks[newDir.Blocks[0].BlockID]
	require.Len(t, entries, 4)
	require.Equal(t, uint64(1), entries[0].UID)
	require.Equal(t, uint32(3), entries[0].Value)
	require.Equal(t, uint64(5), entries[1].UID)
	require.Equal(t, uint32(7), entries[1].Value)
	require.Equal(t, uint64(20), entries[3].UID)
}

func TestBlockMetaFromEntries(t *testing.T) {
	entries := []bm25enc.Entry{
		{UID: 10, Value: 2},
		{UID: 20, Value: 8},
		{UID: 30, Value: 1},
	}
	meta := BlockMetaFromEntries(5, entries)
	require.Equal(t, uint32(5), meta.BlockID)
	require.Equal(t, uint64(10), meta.FirstUID)
	require.Equal(t, uint32(3), meta.Count)
	require.Equal(t, uint32(8), meta.MaxTF)
}

func TestBlockMetaFromEntriesEmpty(t *testing.T) {
	meta := BlockMetaFromEntries(0, nil)
	require.Equal(t, uint32(0), meta.Count)
}

func BenchmarkSplitIntoBlocks(b *testing.B) {
	entries := make([]bm25enc.Entry, 100000)
	for i := range entries {
		entries[i] = bm25enc.Entry{UID: uint64(i*3 + 1), Value: uint32(i%100 + 1)}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		SplitIntoBlocks(entries)
	}
}
