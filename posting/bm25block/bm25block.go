/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

// Package bm25block provides block-based storage for BM25 index data.
//
// Instead of storing all postings for a term in a single blob, this package
// splits them into fixed-size blocks (~128 entries). Each block is stored as
// a separate Badger KV entry, and a lightweight directory indexes the blocks.
//
// This enables:
//   - Selective I/O: queries only read blocks they need
//   - WAND/Block-Max WAND: per-block upper bounds enable early termination
//   - Efficient mutations: only the affected block is rewritten
package bm25block

import (
	"encoding/binary"
	"math"
	"sort"

	"github.com/dgraph-io/dgraph/v25/posting/bm25enc"
)

const (
	// TargetBlockSize is the ideal number of entries per block.
	TargetBlockSize = 128
	// MaxBlockSize is the threshold at which a block is split.
	MaxBlockSize = 256
	// DocLenBlockSize is the target entries per document-length block.
	DocLenBlockSize = 512

	// dirHeaderSize is 4 (blockCount) + 4 (nextID).
	dirHeaderSize = 8
	// dirEntrySize is 8 (firstUID) + 4 (blockID) + 4 (count) + 4 (maxTF).
	dirEntrySize = 20
)

// BlockMeta stores metadata for a single block in a directory.
type BlockMeta struct {
	FirstUID uint64
	BlockID  uint32
	Count    uint32
	MaxTF    uint32
}

// Dir is a block directory for a term's posting list or document-length list.
type Dir struct {
	Blocks []BlockMeta
	NextID uint32 // next available block ID
}

// EncodeDir encodes a directory to bytes. Returns nil for an empty directory.
func EncodeDir(d *Dir) []byte {
	if d == nil || len(d.Blocks) == 0 {
		return nil
	}
	buf := make([]byte, dirHeaderSize+len(d.Blocks)*dirEntrySize)
	binary.BigEndian.PutUint32(buf[0:4], uint32(len(d.Blocks)))
	binary.BigEndian.PutUint32(buf[4:8], d.NextID)
	off := dirHeaderSize
	for _, b := range d.Blocks {
		binary.BigEndian.PutUint64(buf[off:off+8], b.FirstUID)
		binary.BigEndian.PutUint32(buf[off+8:off+12], b.BlockID)
		binary.BigEndian.PutUint32(buf[off+12:off+16], b.Count)
		binary.BigEndian.PutUint32(buf[off+16:off+20], b.MaxTF)
		off += dirEntrySize
	}
	return buf
}

// DecodeDir decodes a directory from bytes. Returns an empty Dir for nil/invalid input.
func DecodeDir(data []byte) *Dir {
	if len(data) < dirHeaderSize {
		return &Dir{}
	}
	count := binary.BigEndian.Uint32(data[0:4])
	nextID := binary.BigEndian.Uint32(data[4:8])
	if int(count)*dirEntrySize+dirHeaderSize > len(data) {
		return &Dir{NextID: nextID}
	}
	blocks := make([]BlockMeta, count)
	off := dirHeaderSize
	for i := uint32(0); i < count; i++ {
		blocks[i] = BlockMeta{
			FirstUID: binary.BigEndian.Uint64(data[off : off+8]),
			BlockID:  binary.BigEndian.Uint32(data[off+8 : off+12]),
			Count:    binary.BigEndian.Uint32(data[off+12 : off+16]),
			MaxTF:    binary.BigEndian.Uint32(data[off+16 : off+20]),
		}
		off += dirEntrySize
	}
	return &Dir{Blocks: blocks, NextID: nextID}
}

// FindBlock returns the index of the block that should contain uid.
// Returns 0 if the directory is empty (caller should create first block).
func (d *Dir) FindBlock(uid uint64) int {
	if len(d.Blocks) == 0 {
		return 0
	}
	// Binary search: find the last block where FirstUID <= uid.
	i := sort.Search(len(d.Blocks), func(i int) bool {
		return d.Blocks[i].FirstUID > uid
	})
	if i > 0 {
		return i - 1
	}
	return 0
}

// AllocBlockID returns the next available block ID and increments the counter.
func (d *Dir) AllocBlockID() uint32 {
	id := d.NextID
	d.NextID++
	return id
}

// UpdateBlockMeta recomputes metadata for the block at index idx from entries.
func (d *Dir) UpdateBlockMeta(idx int, entries []bm25enc.Entry) {
	if idx < 0 || idx >= len(d.Blocks) || len(entries) == 0 {
		return
	}
	d.Blocks[idx].FirstUID = entries[0].UID
	d.Blocks[idx].Count = uint32(len(entries))
	var maxTF uint32
	for _, e := range entries {
		if e.Value > maxTF {
			maxTF = e.Value
		}
	}
	d.Blocks[idx].MaxTF = maxTF
}

// InsertBlockMeta inserts a new block at position idx.
func (d *Dir) InsertBlockMeta(idx int, meta BlockMeta) {
	d.Blocks = append(d.Blocks, BlockMeta{})
	copy(d.Blocks[idx+1:], d.Blocks[idx:])
	d.Blocks[idx] = meta
}

// RemoveBlockMeta removes the block at position idx.
func (d *Dir) RemoveBlockMeta(idx int) {
	if idx < 0 || idx >= len(d.Blocks) {
		return
	}
	d.Blocks = append(d.Blocks[:idx], d.Blocks[idx+1:]...)
}

// SplitIntoBlocks splits a sorted entry slice into blocks of TargetBlockSize.
// Returns a new Dir and a map of blockID -> entries.
func SplitIntoBlocks(entries []bm25enc.Entry) (*Dir, map[uint32][]bm25enc.Entry) {
	if len(entries) == 0 {
		return &Dir{}, nil
	}
	dir := &Dir{}
	blockMap := make(map[uint32][]bm25enc.Entry)

	for i := 0; i < len(entries); i += TargetBlockSize {
		end := i + TargetBlockSize
		if end > len(entries) {
			end = len(entries)
		}
		block := entries[i:end]
		blockID := dir.AllocBlockID()

		var maxTF uint32
		for _, e := range block {
			if e.Value > maxTF {
				maxTF = e.Value
			}
		}

		dir.Blocks = append(dir.Blocks, BlockMeta{
			FirstUID: block[0].UID,
			BlockID:  blockID,
			Count:    uint32(len(block)),
			MaxTF:    maxTF,
		})
		// Make a copy so the caller owns the slice.
		cp := make([]bm25enc.Entry, len(block))
		copy(cp, block)
		blockMap[blockID] = cp
	}
	return dir, blockMap
}

// MergeAllBlocks reads all block entries from a map (keyed by blockID),
// merges them into a single sorted slice, then re-splits into clean blocks.
func MergeAllBlocks(dir *Dir, readBlock func(blockID uint32) []bm25enc.Entry) (*Dir, map[uint32][]bm25enc.Entry) {
	var all []bm25enc.Entry
	for _, bm := range dir.Blocks {
		entries := readBlock(bm.BlockID)
		all = append(all, entries...)
	}
	// Sort by UID and deduplicate (keep last occurrence for same UID).
	sort.Slice(all, func(i, j int) bool { return all[i].UID < all[j].UID })
	deduped := make([]bm25enc.Entry, 0, len(all))
	for i, e := range all {
		if i > 0 && e.UID == all[i-1].UID {
			deduped[len(deduped)-1] = e // overwrite with latest
			continue
		}
		deduped = append(deduped, e)
	}
	// Remove tombstones (Value == 0).
	live := deduped[:0]
	for _, e := range deduped {
		if e.Value > 0 {
			live = append(live, e)
		}
	}
	return SplitIntoBlocks(live)
}

// ComputeUBPre computes the upper-bound pre-IDF BM25 contribution for a block
// given its maxTF and query parameters k and b.
// With dl=0 (best case for scoring): score = (maxTF*(k+1)) / (maxTF + k*(1-b))
func ComputeUBPre(maxTF uint32, k, b float64) float64 {
	if maxTF == 0 {
		return 0
	}
	tf := float64(maxTF)
	return tf * (k + 1) / (tf + k*(1-b))
}

// SuffixMaxUBPre computes suffix maxima of UBPre values for WAND.
// suffixMax[i] = max(ubPre[i], ubPre[i+1], ..., ubPre[n-1])
func SuffixMaxUBPre(dir *Dir, k, b float64) []float64 {
	n := len(dir.Blocks)
	if n == 0 {
		return nil
	}
	suf := make([]float64, n)
	suf[n-1] = ComputeUBPre(dir.Blocks[n-1].MaxTF, k, b)
	for i := n - 2; i >= 0; i-- {
		ub := ComputeUBPre(dir.Blocks[i].MaxTF, k, b)
		suf[i] = math.Max(ub, suf[i+1])
	}
	return suf
}

// BlockMetaFromEntries computes a BlockMeta from entries.
func BlockMetaFromEntries(blockID uint32, entries []bm25enc.Entry) BlockMeta {
	if len(entries) == 0 {
		return BlockMeta{BlockID: blockID}
	}
	var maxTF uint32
	for _, e := range entries {
		if e.Value > maxTF {
			maxTF = e.Value
		}
	}
	return BlockMeta{
		FirstUID: entries[0].UID,
		BlockID:  blockID,
		Count:    uint32(len(entries)),
		MaxTF:    maxTF,
	}
}
