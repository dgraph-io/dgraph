/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

// Package bm25enc provides compact binary encoding for BM25 index data.
//
// Two types of lists share the same format:
//   - Term posting lists: (UID, term-frequency) pairs
//   - Document length lists: (UID, doc-length) pairs
//
// Binary format:
//
//	Header:
//	  [4 bytes] uint32 big-endian: entry count
//	Entries (sorted ascending by UID):
//	  [varint] UID delta from previous (first entry is absolute)
//	  [varint] value (TF or doclen)
package bm25enc

import (
	"encoding/binary"
	"sort"
)

// Entry represents a single (UID, Value) pair in a BM25 posting list.
type Entry struct {
	UID   uint64
	Value uint32
}

// Encode encodes a sorted slice of entries into the compact binary format.
// Entries must be sorted by UID ascending. Returns nil for empty input.
func Encode(entries []Entry) []byte {
	if len(entries) == 0 {
		return nil
	}

	// Pre-allocate: 4 header + ~6 bytes per entry is a reasonable estimate.
	buf := make([]byte, 4, 4+len(entries)*6)
	binary.BigEndian.PutUint32(buf, uint32(len(entries)))

	var tmp [binary.MaxVarintLen64]byte
	var prevUID uint64
	for _, e := range entries {
		delta := e.UID - prevUID
		n := binary.PutUvarint(tmp[:], delta)
		buf = append(buf, tmp[:n]...)
		n = binary.PutUvarint(tmp[:], uint64(e.Value))
		buf = append(buf, tmp[:n]...)
		prevUID = e.UID
	}
	return buf
}

// Decode decodes the binary format into a sorted slice of entries.
// Returns nil for nil/empty input.
func Decode(data []byte) []Entry {
	if len(data) < 4 {
		return nil
	}
	count := binary.BigEndian.Uint32(data[:4])
	if count == 0 {
		return nil
	}

	entries := make([]Entry, 0, count)
	pos := 4
	var prevUID uint64
	for i := uint32(0); i < count; i++ {
		delta, n := binary.Uvarint(data[pos:])
		if n <= 0 {
			break
		}
		pos += n

		val, n := binary.Uvarint(data[pos:])
		if n <= 0 {
			break
		}
		pos += n

		uid := prevUID + delta
		entries = append(entries, Entry{UID: uid, Value: uint32(val)})
		prevUID = uid
	}
	return entries
}

// Upsert inserts or updates the entry for uid in a sorted entries slice.
// Returns the new sorted slice.
func Upsert(entries []Entry, uid uint64, value uint32) []Entry {
	i := sort.Search(len(entries), func(i int) bool { return entries[i].UID >= uid })
	if i < len(entries) && entries[i].UID == uid {
		entries[i].Value = value
		return entries
	}
	// Insert at position i.
	entries = append(entries, Entry{})
	copy(entries[i+1:], entries[i:])
	entries[i] = Entry{UID: uid, Value: value}
	return entries
}

// Remove removes the entry for uid from a sorted entries slice.
// Returns the new slice (may be shorter).
func Remove(entries []Entry, uid uint64) []Entry {
	i := sort.Search(len(entries), func(i int) bool { return entries[i].UID >= uid })
	if i < len(entries) && entries[i].UID == uid {
		return append(entries[:i], entries[i+1:]...)
	}
	return entries
}

// Search returns the value for uid using binary search, and whether it was found.
func Search(entries []Entry, uid uint64) (uint32, bool) {
	i := sort.Search(len(entries), func(i int) bool { return entries[i].UID >= uid })
	if i < len(entries) && entries[i].UID == uid {
		return entries[i].Value, true
	}
	return 0, false
}

// UIDs extracts just the UIDs from entries as a uint64 slice.
func UIDs(entries []Entry) []uint64 {
	uids := make([]uint64, len(entries))
	for i, e := range entries {
		uids[i] = e.UID
	}
	return uids
}

// EncodeStats encodes BM25 corpus statistics (docCount, totalTerms) as 16 bytes.
func EncodeStats(docCount, totalTerms uint64) []byte {
	buf := make([]byte, 16)
	binary.BigEndian.PutUint64(buf[0:8], docCount)
	binary.BigEndian.PutUint64(buf[8:16], totalTerms)
	return buf
}

// DecodeStats decodes BM25 corpus statistics. Returns (0,0) for invalid input.
func DecodeStats(data []byte) (docCount, totalTerms uint64) {
	if len(data) != 16 {
		return 0, 0
	}
	return binary.BigEndian.Uint64(data[0:8]), binary.BigEndian.Uint64(data[8:16])
}
